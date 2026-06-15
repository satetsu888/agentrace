import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import * as http from "node:http";
import type { AddressInfo } from "node:net";

interface Recorded {
  session_id: string;
  transcript_lines: Array<Record<string, unknown>>;
}

describe("send/worker", () => {
  let tmpHome: string;
  let projectDir: string;
  let transcriptPath: string;
  let prevHome: string | undefined;
  let prevUserProfile: string | undefined;
  let server: http.Server;
  let baseUrl: string;
  let received: Recorded[];
  let respondStatus = 200;
  const sockets = new Set<import("node:net").Socket>();

  // Imported fresh per test so cursor/lock modules bind to the temp HOME.
  let runWorker: (
    p: { sessionId: string; transcriptPath: string; projectDir?: string },
    lockOptions?: { pollIntervalMs?: number; maxWaitMs?: number }
  ) => Promise<void>;
  let getCursor: (sid: string) => number;
  let holderDir: (sid: string) => string;
  let acquireHolder: (sid: string) => boolean;
  let saveCursor: (sid: string, n: number) => void;
  let releaseSessionLock: (sid: string) => void;

  function writeLines(lines: Array<Record<string, unknown>>): void {
    fs.writeFileSync(
      transcriptPath,
      lines.map((l) => JSON.stringify(l)).join("\n") + "\n"
    );
  }

  function lineRange(start: number, end: number): Array<Record<string, unknown>> {
    const out: Array<Record<string, unknown>> = [];
    for (let i = start; i < end; i++) {
      out.push({ type: "user", uuid: `u${i}`, n: i });
    }
    return out;
  }

  beforeEach(async () => {
    tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-wkhome-"));
    projectDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-wkproj-"));
    transcriptPath = path.join(projectDir, "transcript.jsonl");
    prevHome = process.env.HOME;
    prevUserProfile = process.env.USERPROFILE;
    process.env.HOME = tmpHome;
    process.env.USERPROFILE = tmpHome;

    received = [];
    respondStatus = 200;
    server = http.createServer((req, res) => {
      let body = "";
      req.on("data", (c) => (body += c));
      req.on("end", () => {
        if (respondStatus === 200) {
          received.push(JSON.parse(body));
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ ok: true, events_created: 1 }));
        } else {
          res.writeHead(respondStatus);
          res.end("error");
        }
      });
    });
    server.on("connection", (s) => {
      sockets.add(s);
      s.on("close", () => sockets.delete(s));
    });
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
    baseUrl = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;

    vi.resetModules();
    const manager = await import("../config/manager.js");
    manager.saveLocalConfig(projectDir, {
      server_url: baseUrl,
      api_key: "agtr_test",
    });
    ({ runWorker } = await import("./worker.js"));
    ({ getCursor, saveCursor } = await import("../config/cursor.js"));
    ({ holderDir, acquireHolder, releaseSessionLock } = await import("./lock.js"));
  });

  afterEach(async () => {
    if (prevHome === undefined) delete process.env.HOME;
    else process.env.HOME = prevHome;
    if (prevUserProfile === undefined) delete process.env.USERPROFILE;
    else process.env.USERPROFILE = prevUserProfile;
    for (const s of sockets) s.destroy();
    sockets.clear();
    await new Promise<void>((r) => server.close(() => r()));
    fs.rmSync(tmpHome, { recursive: true, force: true });
    fs.rmSync(projectDir, { recursive: true, force: true });
  });

  it("advances the cursor and releases the lock on a successful send", async () => {
    writeLines(lineRange(0, 3));

    await runWorker({ sessionId: "s1", transcriptPath, projectDir });

    expect(getCursor("s1")).toBe(3);
    expect(received).toHaveLength(1);
    expect(received[0].transcript_lines).toHaveLength(3);
    expect(fs.existsSync(holderDir("s1"))).toBe(false);
  });

  it("does not advance the cursor on a failed send (next worker can pick it up)", async () => {
    respondStatus = 500;
    writeLines(lineRange(0, 3));

    await runWorker({ sessionId: "s1", transcriptPath, projectDir });

    expect(getCursor("s1")).toBe(0);
    expect(fs.existsSync(holderDir("s1"))).toBe(false);
  });

  it("exits without sending when the lock is dropped (holder + waiter both taken)", async () => {
    // Occupy holder and waiting slot so the worker is dropped.
    acquireHolder("s1");
    const { acquireWaiting } = await import("./lock.js");
    acquireWaiting("s1");
    writeLines(lineRange(0, 3));

    await runWorker({ sessionId: "s1", transcriptPath, projectDir });

    expect(received).toHaveLength(0);
    expect(getCursor("s1")).toBe(0);
  });

  it("a waiter reads cursor→tail after the holder releases, covering lines added while waiting", async () => {
    // Holder is busy; the worker must wait, then read the latest tail.
    acquireHolder("s1");
    writeLines(lineRange(0, 2));

    const waiter = runWorker(
      { sessionId: "s1", transcriptPath, projectDir },
      { pollIntervalMs: 10, maxWaitMs: 2000 }
    );

    // Let the worker reach the waiting state, then simulate the holder finishing
    // its own send (cursor → 2) and appending two more lines before releasing.
    await new Promise((r) => setTimeout(r, 50));
    writeLines(lineRange(0, 4));
    saveCursor("s1", 2);
    releaseSessionLock("s1");

    await waiter;

    expect(getCursor("s1")).toBe(4);
    expect(received).toHaveLength(1);
    expect(received[0].transcript_lines).toHaveLength(2); // lines 2..4 only
    expect((received[0].transcript_lines[0] as { n: number }).n).toBe(2);
  });
});
