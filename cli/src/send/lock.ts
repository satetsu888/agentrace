import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";

export const MAX_LOCK_MS = 60_000;
export const LOCK_POLL_INTERVAL_MS = 100;
export const MAX_WAITER_MS = MAX_LOCK_MS;

export interface LockMeta {
  pid?: number;
  startedAt: number;
  instanceId?: string;
}

let staleCounter = 0;
let instanceCounter = 0;

export function locksDir(): string {
  return path.join(os.homedir(), ".agentrace", "locks");
}

export function holderDir(sid: string): string {
  return path.join(locksDir(), sid);
}

export function waitingDir(sid: string): string {
  return path.join(locksDir(), `${sid}.waiting`);
}

function isPidAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (err) {
    return (err as NodeJS.ErrnoException).code === "EPERM";
  }
}

export function readMeta(dir: string): LockMeta | null {
  let dirStat: fs.Stats;
  try {
    dirStat = fs.statSync(dir);
  } catch {
    return null;
  }
  try {
    const raw = fs.readFileSync(path.join(dir, "meta.json"), "utf-8");
    const parsed = JSON.parse(raw) as LockMeta;
    if (typeof parsed.startedAt === "number") {
      return parsed;
    }
  } catch {
    // meta.json not written yet; identify the dir by its mtime instead.
  }
  return { startedAt: dirStat.mtimeMs };
}

function isStale(meta: LockMeta): boolean {
  if (typeof meta.pid === "number" && !isPidAlive(meta.pid)) {
    return true;
  }
  return Date.now() - meta.startedAt > MAX_LOCK_MS;
}

function sameInstance(a: LockMeta, b: LockMeta): boolean {
  if (a.instanceId !== undefined || b.instanceId !== undefined) {
    return a.instanceId === b.instanceId;
  }
  return a.pid === b.pid && a.startedAt === b.startedAt;
}

// Remove a stale dir only if it is still the same instance `observed` earlier,
// re-checking right before the rename so a freshly re-acquired holder is not
// removed. Returns true only when the observed instance was removed.
export function evictStale(dir: string, observed: LockMeta): boolean {
  const current = readMeta(dir);
  if (!current || !sameInstance(current, observed) || !isStale(current)) {
    return false;
  }
  const target = `${dir}.stale.${process.pid}.${staleCounter++}`;
  try {
    fs.renameSync(dir, target);
  } catch {
    return false;
  }
  fs.rmSync(target, { recursive: true, force: true });
  return true;
}

function tryMkdir(dir: string): boolean {
  fs.mkdirSync(locksDir(), { recursive: true });
  try {
    fs.mkdirSync(dir);
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "EEXIST") {
      return false;
    }
    throw err;
  }
  const meta: LockMeta = {
    pid: process.pid,
    startedAt: Date.now(),
    instanceId: `${process.pid}-${Date.now()}-${instanceCounter++}`,
  };
  fs.writeFileSync(path.join(dir, "meta.json"), JSON.stringify(meta));
  return true;
}

function takeSlot(dir: string): boolean {
  if (tryMkdir(dir)) {
    return true;
  }
  const observed = readMeta(dir);
  if (observed && isStale(observed) && evictStale(dir, observed)) {
    return tryMkdir(dir);
  }
  return false;
}

export function acquireHolder(sid: string): boolean {
  return takeSlot(holderDir(sid));
}

export function acquireWaiting(sid: string): boolean {
  return takeSlot(waitingDir(sid));
}

// Remove a slot only if we still own it, so a worker whose lock was stale-evicted
// and re-acquired by another process never deletes that new holder's lock.
function removeIfOwned(dir: string): void {
  const meta = readMeta(dir);
  if (meta && meta.pid === process.pid) {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

export function releaseSessionLock(sid: string): void {
  removeIfOwned(holderDir(sid));
}

export function releaseWaiting(sid: string): void {
  removeIfOwned(waitingDir(sid));
}

export type AcquireOutcome = "acquired" | "dropped";

export interface AcquireOptions {
  pollIntervalMs?: number;
  maxWaitMs?: number;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export async function acquireSessionLock(
  sid: string,
  opts: AcquireOptions = {}
): Promise<AcquireOutcome> {
  if (acquireHolder(sid)) {
    return "acquired";
  }
  if (!acquireWaiting(sid)) {
    return "dropped";
  }

  const pollIntervalMs = opts.pollIntervalMs ?? LOCK_POLL_INTERVAL_MS;
  const maxWaitMs = opts.maxWaitMs ?? MAX_WAITER_MS;
  const deadline = Date.now() + maxWaitMs;
  try {
    while (Date.now() < deadline) {
      await sleep(pollIntervalMs);
      if (acquireHolder(sid)) {
        return "acquired";
      }
    }
    return "dropped";
  } finally {
    releaseWaiting(sid);
  }
}
