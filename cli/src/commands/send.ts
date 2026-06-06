import { execSync, spawn } from "child_process";
import { loadConfigWithFallback, getSendMode } from "../config/manager.js";
import { getNewLines, saveCursor, hasCursor } from "../config/cursor.js";
import { sendIngest } from "../utils/http.js";
import { WORKER_ENV } from "../send/worker.js";
import {
  findSessionFile,
  extractCwdFromTranscript,
} from "../utils/session-finder.js";

interface HookInput {
  session_id?: string;
  transcript_path?: string;
  cwd?: string;
  hook_event_name?: string;
}

interface SendTranscriptParams {
  sessionId: string;
  transcriptPath: string;
  cwd?: string;
  isHook: boolean;
}

export interface RunSendParams {
  sessionId: string;
  transcriptPath: string;
  cwd?: string;
}

export type SendOutcome =
  | { status: "no-config" }
  | { status: "no-lines" }
  | { status: "no-valid-lines" }
  | { status: "sent"; lineCount: number }
  | { status: "error"; error: string };

// Event types that should not be sent to the server (high-volume, not needed for display)
const SKIPPED_EVENT_TYPES = ["progress", "file-history-snapshot"];

function getGitRemoteUrl(cwd: string): string | null {
  try {
    const url = execSync("git remote get-url origin", {
      cwd,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
    return url || null;
  } catch {
    return null; // Not a git repo or no remote
  }
}

function getGitBranch(cwd: string): string | null {
  try {
    const branch = execSync("git branch --show-current", {
      cwd,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
    return branch || null;
  } catch {
    return null;
  }
}

/**
 * Send the cursor diff to the server and advance the cursor on success.
 * Returns an outcome instead of exiting so callers control reporting and,
 * for the async worker, lock release.
 */
export async function runSend(params: RunSendParams): Promise<SendOutcome> {
  const { sessionId, transcriptPath, cwd } = params;

  const config = loadConfigWithFallback(cwd);
  if (!config) {
    return { status: "no-config" };
  }

  const { lines, totalLineCount } = getNewLines(transcriptPath, sessionId);
  if (lines.length === 0) {
    return { status: "no-lines" };
  }

  // Parse JSONL lines and filter out skipped event types
  const transcriptLines: unknown[] = [];
  for (const line of lines) {
    try {
      const parsed = JSON.parse(line) as Record<string, unknown>;
      if (typeof parsed.type === "string" && SKIPPED_EVENT_TYPES.includes(parsed.type)) {
        continue;
      }
      transcriptLines.push(parsed);
    } catch {
      // Skip invalid JSON lines
    }
  }

  if (transcriptLines.length === 0) {
    return { status: "no-valid-lines" };
  }

  // Detect subagent (Task tool) sessions from first transcript line
  const firstLine = transcriptLines[0] as Record<string, unknown> | undefined;
  const isSidechain = firstLine?.isSidechain === true;
  const agentId = typeof firstLine?.agentId === "string" ? firstLine.agentId : undefined;
  // For subagents, sessionId in transcript is the parent session ID
  const parentSessionId = isSidechain && typeof firstLine?.sessionId === "string"
    ? firstLine.sessionId
    : undefined;

  // Generate title for subagents from first user message
  let subagentTitle: string | undefined;
  if (isSidechain) {
    const firstUserMsg = transcriptLines.find(
      (line): line is Record<string, unknown> =>
        typeof line === "object" &&
        line !== null &&
        (line as Record<string, unknown>).type === "user"
    );
    if (firstUserMsg?.message && typeof (firstUserMsg.message as Record<string, unknown>)?.content === "string") {
      const content = (firstUserMsg.message as Record<string, unknown>).content as string;
      subagentTitle = content.slice(0, 100) + (content.length > 100 ? "..." : "");
    }
  }

  // Extract git info only on first send (when cursor doesn't exist yet)
  let gitRemoteUrl: string | undefined;
  let gitBranch: string | undefined;
  if (cwd && !hasCursor(sessionId)) {
    gitRemoteUrl = getGitRemoteUrl(cwd) ?? undefined;
    gitBranch = getGitBranch(cwd) ?? undefined;
  }

  const result = await sendIngest(
    {
      session_id: sessionId,
      transcript_lines: transcriptLines,
      cwd: cwd,
      git_remote_url: gitRemoteUrl,
      git_branch: gitBranch,
      parent_session_id: parentSessionId,
      agent_id: agentId,
      is_sidechain: isSidechain || undefined,
      title: subagentTitle,
    },
    cwd
  );

  if (!result.ok) {
    return { status: "error", error: result.error ?? "unknown error" };
  }

  // Update cursor only on success so a failed send is retried by the next fire.
  saveCursor(sessionId, totalLineCount);
  return { status: "sent", lineCount: transcriptLines.length };
}

/**
 * Send wrapper for the synchronous hook path and manual invocation.
 * Maps the outcome to logging and the existing exit-code contract
 * (hook: always exit 0; manual: exit 1 on error).
 */
async function sendTranscript(params: SendTranscriptParams): Promise<void> {
  const { sessionId, transcriptPath, cwd, isHook } = params;

  const outcome = await runSend({ sessionId, transcriptPath, cwd });

  let exitCode = 0;
  switch (outcome.status) {
    case "no-config":
      console.error(
        "[agentrace] Warning: Config not found. Run 'npx agentrace init' first."
      );
      exitCode = isHook ? 0 : 1;
      break;
    case "no-lines":
      if (!isHook) {
        console.log("[agentrace] No new lines to send.");
      }
      break;
    case "no-valid-lines":
      if (!isHook) {
        console.log("[agentrace] No valid transcript lines to send.");
      }
      break;
    case "sent":
      if (!isHook) {
        console.log(
          `[agentrace] Sent ${outcome.lineCount} lines for session ${sessionId}`
        );
      }
      break;
    case "error":
      console.error(`[agentrace] Warning: ${outcome.error}`);
      exitCode = isHook ? 0 : 1;
      break;
  }
  process.exit(exitCode);
}

/**
 * Hook-based send command.
 * Reads session info from stdin (provided by Claude Code hooks).
 */
export async function sendCommand(): Promise<void> {
  // Read stdin
  let input = "";
  try {
    input = await readStdin();
  } catch {
    console.error("[agentrace] Warning: Failed to read stdin");
    process.exit(0);
  }

  if (!input.trim()) {
    console.error("[agentrace] Warning: Empty input");
    process.exit(0);
  }

  // Parse JSON
  let data: HookInput;
  try {
    data = JSON.parse(input);
  } catch {
    console.error("[agentrace] Warning: Invalid JSON input");
    process.exit(0);
  }

  const sessionId = data.session_id;
  const transcriptPath = data.transcript_path;

  if (!sessionId || !transcriptPath) {
    console.error("[agentrace] Warning: Missing session_id or transcript_path");
    process.exit(0);
  }

  // Use CLAUDE_PROJECT_DIR (stable project root) instead of cwd (can change during builds)
  const projectDir = process.env.CLAUDE_PROJECT_DIR || data.cwd;

  // Async mode: hand off to a detached worker and return immediately, keeping
  // the HTTPS send off the hook's critical path. The 10s UserPromptSubmit wait
  // is sync-only. If the worker cannot be spawned, fall back to a sync send so
  // the hook neither crashes nor drops the batch.
  if (getSendMode(loadConfigWithFallback(projectDir)) === "async") {
    try {
      spawnWorker({ sessionId, transcriptPath, projectDir });
      process.exit(0);
    } catch {
      // fall through to the synchronous send below
    }
  }

  // For UserPromptSubmit, wait for transcript to be written
  // (Claude hasn't started processing yet, so transcript may not be updated)
  if (data.hook_event_name === "UserPromptSubmit") {
    await sleep(10000);
  }

  await sendTranscript({
    sessionId,
    transcriptPath,
    cwd: projectDir,
    isHook: true,
  });
}

function spawnWorker(payload: {
  sessionId: string;
  transcriptPath: string;
  projectDir?: string;
}): void {
  const child = spawn(
    process.execPath,
    [...process.execArgv, process.argv[1], "__send-worker"],
    {
      detached: true,
      stdio: "ignore",
      env: {
        ...process.env,
        [WORKER_ENV.sessionId]: payload.sessionId,
        [WORKER_ENV.transcriptPath]: payload.transcriptPath,
        [WORKER_ENV.projectDir]: payload.projectDir ?? "",
      },
    }
  );
  child.on("error", () => {});
  child.unref();
}

/**
 * Manual send command.
 * Finds session file by ID and sends to server.
 */
export async function sendManualCommand(options: {
  sessionId: string;
}): Promise<void> {
  const { sessionId } = options;

  // Find session file
  const transcriptPath = findSessionFile(sessionId);
  if (!transcriptPath) {
    console.error(
      `[agentrace] Error: Session file not found for ID: ${sessionId}`
    );
    console.error("  Searched in: ~/.claude/projects/");
    process.exit(1);
  }

  // Extract cwd from transcript
  const cwd = extractCwdFromTranscript(transcriptPath) ?? undefined;

  console.log(`[agentrace] Found session file: ${transcriptPath}`);

  await sendTranscript({
    sessionId,
    transcriptPath,
    cwd,
    isHook: false,
  });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function readStdin(): Promise<string> {
  return new Promise((resolve, reject) => {
    let data = "";

    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => {
      data += chunk;
    });
    process.stdin.on("end", () => {
      resolve(data);
    });
    process.stdin.on("error", reject);

    // Set timeout to avoid hanging
    setTimeout(() => {
      resolve(data);
    }, 5000);
  });
}
