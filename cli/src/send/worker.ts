import { acquireSessionLock, releaseSessionLock, type AcquireOptions } from "./lock.js";
import { runSend } from "../commands/send.js";

export interface WorkerPayload {
  sessionId: string;
  transcriptPath: string;
  projectDir?: string;
}

export async function runWorker(
  payload: WorkerPayload,
  lockOptions?: AcquireOptions
): Promise<void> {
  const { sessionId, transcriptPath, projectDir } = payload;

  const outcome = await acquireSessionLock(sessionId, lockOptions);
  if (outcome === "dropped") {
    return;
  }

  const release = () => releaseSessionLock(sessionId);
  process.once("exit", release);
  try {
    await runSend({ sessionId, transcriptPath, cwd: projectDir });
  } finally {
    process.removeListener("exit", release);
    release();
  }
}

export async function workerMain(): Promise<void> {
  const sessionId = process.env.AGENTRACE_WORKER_SESSION_ID;
  const transcriptPath = process.env.AGENTRACE_WORKER_TRANSCRIPT_PATH;
  const projectDir = process.env.AGENTRACE_WORKER_PROJECT_DIR || undefined;

  if (!sessionId || !transcriptPath) {
    return;
  }

  await runWorker({ sessionId, transcriptPath, projectDir });
}
