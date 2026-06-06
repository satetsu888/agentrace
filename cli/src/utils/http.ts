import { fetch } from "undici";
import { loadConfigWithFallback } from "../config/manager.js";
import { createDispatcher } from "./proxy.js";

export interface IngestPayload {
  session_id: string;
  transcript_lines: unknown[];
  cwd?: string;
  git_remote_url?: string;
  git_branch?: string;
  // Subagent (Task tool) fields
  parent_session_id?: string;
  agent_id?: string;
  is_sidechain?: boolean;
  title?: string;
}

export interface IngestResponse {
  ok: boolean;
  events_created?: number;
  error?: string;
}

/**
 * Default send timeout for ingest requests (~30s, tunable).
 * Bounds how long a worker can hold a per-session lock while a send hangs,
 * which is a precondition for self-recovering liveness (HC-6).
 * Must stay below MAX_LOCK_MS in send/lock.ts.
 */
export const SEND_TIMEOUT_MS = 30_000;

function getSendTimeoutMs(): number {
  const override = Number(process.env.AGENTRACE_SEND_TIMEOUT_MS);
  return Number.isFinite(override) && override > 0 ? override : SEND_TIMEOUT_MS;
}

export interface WebSessionResponse {
  url: string;
  expires_at: string;
}

function getBaseUrl(config: { server_url: string }): string {
  return config.server_url.replace(/\/+$/, '');
}

export async function sendIngest(
  payload: IngestPayload,
  projectDir?: string
): Promise<IngestResponse> {
  const config = loadConfigWithFallback(projectDir);
  if (!config) {
    return { ok: false, error: "Config not found" };
  }

  const url = `${getBaseUrl(config)}/api/ingest`;

  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${config.api_key}`,
      },
      body: JSON.stringify(payload),
      dispatcher: createDispatcher(projectDir),
      // Bound the request so a hung send cannot hold a session lock forever.
      // Timeout/abort throws and is normalized to { ok: false } by the catch below.
      signal: AbortSignal.timeout(getSendTimeoutMs()),
    });

    if (!response.ok) {
      const text = await response.text();
      return { ok: false, error: `HTTP ${response.status}: ${text}` };
    }

    return (await response.json()) as IngestResponse;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { ok: false, error: message };
  }
}

export async function createWebSession(
  projectDir?: string
): Promise<
  { ok: true; data: WebSessionResponse } | { ok: false; error: string }
> {
  const config = loadConfigWithFallback(projectDir);
  if (!config) {
    return { ok: false, error: "Config not found. Run 'agentrace init' first." };
  }

  const url = `${getBaseUrl(config)}/api/auth/web-session`;

  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${config.api_key}`,
      },
      dispatcher: createDispatcher(projectDir),
    });

    if (!response.ok) {
      const text = await response.text();
      return { ok: false, error: `HTTP ${response.status}: ${text}` };
    }

    const data = (await response.json()) as WebSessionResponse;
    return { ok: true, data };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { ok: false, error: message };
  }
}
