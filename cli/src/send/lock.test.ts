import { describe, it, expect, beforeEach, afterEach } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import {
  acquireHolder,
  acquireSessionLock,
  releaseSessionLock,
  acquireWaiting,
  releaseWaiting,
  evictStale,
  readMeta,
  locksDir,
  holderDir,
  waitingDir,
  MAX_LOCK_MS,
} from "./lock.js";
import { SEND_TIMEOUT_MS } from "../utils/http.js";

const SID = "session-abc";

function writeMeta(dir: string, meta: { pid?: number; startedAt: number }): void {
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, "meta.json"), JSON.stringify(meta));
}

// A pid that is essentially guaranteed not to exist.
const DEAD_PID = 2147483646;

describe("send/lock", () => {
  let tmpHome: string;
  let prevHome: string | undefined;
  let prevUserProfile: string | undefined;

  beforeEach(() => {
    tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-lockhome-"));
    prevHome = process.env.HOME;
    prevUserProfile = process.env.USERPROFILE;
    process.env.HOME = tmpHome;
    process.env.USERPROFILE = tmpHome;
  });

  afterEach(() => {
    if (prevHome === undefined) delete process.env.HOME;
    else process.env.HOME = prevHome;
    if (prevUserProfile === undefined) delete process.env.USERPROFILE;
    else process.env.USERPROFILE = prevUserProfile;
    fs.rmSync(tmpHome, { recursive: true, force: true });
  });

  it("MAX_LOCK_MS is greater than SEND_TIMEOUT_MS (do not evict a slow but live holder)", () => {
    expect(MAX_LOCK_MS).toBeGreaterThan(SEND_TIMEOUT_MS);
  });

  it("locksDir is under ~/.agentrace/locks", () => {
    expect(locksDir()).toBe(path.join(tmpHome, ".agentrace", "locks"));
  });

  describe("mutual exclusion", () => {
    it("first acquire succeeds, second acquire on same sid fails", () => {
      expect(acquireHolder(SID)).toBe(true);
      expect(acquireHolder(SID)).toBe(false);
    });

    it("writes meta.json with pid and startedAt on acquire", () => {
      acquireHolder(SID);
      const metaPath = path.join(holderDir(SID), "meta.json");
      expect(fs.existsSync(metaPath)).toBe(true);
      const meta = JSON.parse(fs.readFileSync(metaPath, "utf-8"));
      expect(meta.pid).toBe(process.pid);
      expect(typeof meta.startedAt).toBe("number");
    });

    it("can re-acquire after release", () => {
      expect(acquireHolder(SID)).toBe(true);
      releaseSessionLock(SID);
      expect(fs.existsSync(holderDir(SID))).toBe(false);
      expect(acquireHolder(SID)).toBe(true);
    });
  });

  describe("waiting slot (holder 1 + waiting 1, max 2)", () => {
    it("waiting slot can be taken while holder is held, third is rejected", () => {
      expect(acquireHolder(SID)).toBe(true);
      expect(acquireWaiting(SID)).toBe(true);
      // holder + waiting both occupied -> third caller (another waiting) fails
      expect(acquireWaiting(SID)).toBe(false);
    });

    it("can re-take waiting slot after release", () => {
      acquireHolder(SID);
      expect(acquireWaiting(SID)).toBe(true);
      releaseWaiting(SID);
      expect(acquireWaiting(SID)).toBe(true);
    });
  });

  describe("staleness / self-recovery", () => {
    it("evicts a holder whose pid is dead (immediate)", () => {
      writeMeta(holderDir(SID), { pid: DEAD_PID, startedAt: Date.now() });
      expect(acquireHolder(SID)).toBe(true);
      // new holder meta belongs to us
      const meta = JSON.parse(
        fs.readFileSync(path.join(holderDir(SID), "meta.json"), "utf-8")
      );
      expect(meta.pid).toBe(process.pid);
    });

    it("evicts a holder whose age exceeds MAX_LOCK_MS even if pid is alive", () => {
      writeMeta(holderDir(SID), {
        pid: process.pid, // alive
        startedAt: Date.now() - (MAX_LOCK_MS + 5_000),
      });
      expect(acquireHolder(SID)).toBe(true);
    });

    it("does NOT evict a live, recent holder (no meta yet = just-acquired, not stale)", () => {
      // holder dir exists but meta not written yet, and dir is fresh
      fs.mkdirSync(holderDir(SID), { recursive: true });
      expect(acquireHolder(SID)).toBe(false);
    });

    it("evicts a meta-less holder dir once it is older than MAX_LOCK_MS (crash backstop)", () => {
      fs.mkdirSync(holderDir(SID), { recursive: true });
      // backdate the directory's mtime beyond MAX_LOCK_MS
      const old = new Date(Date.now() - (MAX_LOCK_MS + 5_000));
      fs.utimesSync(holderDir(SID), old, old);
      expect(acquireHolder(SID)).toBe(true);
    });

    it("reclaims a stale waiting slot (crashed waiter) on next acquireWaiting", () => {
      acquireHolder(SID);
      writeMeta(waitingDir(SID), { pid: DEAD_PID, startedAt: Date.now() });
      expect(acquireWaiting(SID)).toBe(true);
      const meta = JSON.parse(
        fs.readFileSync(path.join(waitingDir(SID), "meta.json"), "utf-8")
      );
      expect(meta.pid).toBe(process.pid);
    });
  });

  describe("release only removes a lock we own", () => {
    it("does not remove a holder owned by another process", () => {
      writeMeta(holderDir(SID), { pid: DEAD_PID, startedAt: Date.now() });
      releaseSessionLock(SID);
      expect(fs.existsSync(holderDir(SID))).toBe(true);
    });

    it("removes a holder we acquired ourselves", () => {
      expect(acquireHolder(SID)).toBe(true);
      releaseSessionLock(SID);
      expect(fs.existsSync(holderDir(SID))).toBe(false);
    });
  });

  describe("acquireSessionLock (high-level)", () => {
    it("returns 'acquired' when the holder is free", async () => {
      await expect(acquireSessionLock(SID)).resolves.toBe("acquired");
      expect(fs.existsSync(holderDir(SID))).toBe(true);
    });

    it("returns 'dropped' when holder and waiting are both occupied", async () => {
      acquireHolder(SID); // holder taken by a live (this) process
      acquireWaiting(SID); // waiting taken too
      await expect(
        acquireSessionLock(SID, { pollIntervalMs: 5, maxWaitMs: 50 })
      ).resolves.toBe("dropped");
    });

    it("promotes the waiter to holder once the holder releases, freeing the waiting slot", async () => {
      acquireHolder(SID); // someone else holds it
      const waiterPromise = acquireSessionLock(SID, {
        pollIntervalMs: 5,
        maxWaitMs: 2_000,
      });
      // release the holder shortly after so the waiter can promote
      setTimeout(() => releaseSessionLock(SID), 30);
      await expect(waiterPromise).resolves.toBe("acquired");
      expect(fs.existsSync(holderDir(SID))).toBe(true);
      expect(fs.existsSync(waitingDir(SID))).toBe(false);
    });
  });

  describe("eviction never clobbers a refreshed holder", () => {
    it("aborts eviction when the holder was replaced since it was observed as stale", () => {
      // A stale holder is observed (dead pid).
      writeMeta(holderDir(SID), { pid: DEAD_PID, startedAt: Date.now() });
      const observed = readMeta(holderDir(SID))!;

      // Concurrently, that stale holder is reclaimed and replaced by a fresh,
      // live holder (us) with a different instanceId.
      fs.rmSync(holderDir(SID), { recursive: true, force: true });
      expect(acquireHolder(SID)).toBe(true);
      const freshMeta = readMeta(holderDir(SID))!;
      expect(freshMeta.instanceId).not.toBe(observed.instanceId);

      // An evictor still holding the STALE observation must not remove the fresh
      // holder's dir.
      expect(evictStale(holderDir(SID), observed)).toBe(false);
      expect(readMeta(holderDir(SID))).toEqual(freshMeta);
      expect(fs.existsSync(holderDir(SID))).toBe(true);
    });

    it("evicts when the observed stale instance is still the current one", () => {
      writeMeta(holderDir(SID), { pid: DEAD_PID, startedAt: Date.now() });
      const observed = readMeta(holderDir(SID))!;
      expect(evictStale(holderDir(SID), observed)).toBe(true);
      expect(fs.existsSync(holderDir(SID))).toBe(false);
    });

    it("treats a meta-less dir with a different mtime as a different instance", () => {
      // Observe a meta-less, aged-out holder (crash-before-meta backstop).
      fs.mkdirSync(holderDir(SID), { recursive: true });
      const old1 = new Date(Date.now() - (MAX_LOCK_MS + 10_000));
      fs.utimesSync(holderDir(SID), old1, old1);
      const observed = readMeta(holderDir(SID))!;
      expect(observed.instanceId).toBeUndefined();

      // Replace it with a different meta-less dir (different mtime, still stale).
      fs.rmSync(holderDir(SID), { recursive: true, force: true });
      fs.mkdirSync(holderDir(SID), { recursive: true });
      const old2 = new Date(Date.now() - (MAX_LOCK_MS + 3_000));
      fs.utimesSync(holderDir(SID), old2, old2);

      // The mtime-based identity differs → eviction must abort.
      expect(evictStale(holderDir(SID), observed)).toBe(false);
      expect(fs.existsSync(holderDir(SID))).toBe(true);
    });
  });

  describe("concurrent stale takeover yields a unique holder", () => {
    it("only one of N processes becomes the holder of a stale lock", async () => {
      // Pre-create a stale holder (dead pid).
      writeMeta(holderDir(SID), { pid: DEAD_PID, startedAt: Date.now() });

      const here = path.dirname(fileURLToPath(import.meta.url));
      const racerPath = path.join(here, "__lock_racer__.ts");
      // Each racer reports its result, then stays alive holding the lock so a
      // winner's pid does not die and let the next racer reclaim it as stale.
      // The unique-holder guarantee then rests purely on atomic mkdir.
      fs.writeFileSync(
        racerPath,
        `import { acquireHolder } from "./lock.js";\n` +
          `const got = acquireHolder(process.env.SID!);\n` +
          `process.stdout.write(got ? "ACQUIRED" : "NOPE");\n` +
          `setTimeout(() => process.exit(0), 1500);\n`
      );

      try {
        const N = 5;
        const runs = Array.from({ length: N }, () =>
          new Promise<string>((resolve) => {
            // Launch each racer as a separate OS process under tsx so the
            // mkdir/rename race is genuinely cross-process, not cooperative.
            const child = spawn(
              process.execPath,
              ["--import", "tsx", racerPath],
              { env: { ...process.env, SID }, stdio: ["ignore", "pipe", "ignore"] }
            );
            let out = "";
            child.stdout.on("data", (c) => (out += c.toString()));
            child.on("close", () => resolve(out.trim()));
          })
        );
        const results = await Promise.all(runs);
        const winners = results.filter((r) => r === "ACQUIRED").length;
        expect(winners).toBe(1);
      } finally {
        fs.rmSync(racerPath, { force: true });
      }
    }, 30_000);
  });
});
