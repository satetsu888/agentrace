import { describe, it, expect, beforeEach, afterEach } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import * as http from "node:http";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import type { AddressInfo } from "node:net";

const indexPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../index.ts"
);

describe("async send handoff (sendCommand → detached worker)", () => {
  let tmpHome: string;
  let projectDir: string;
  let server: http.Server;
  let baseUrl: string;
  let received: string[];
  const sockets = new Set<import("node:net").Socket>();

  beforeEach(async () => {
    tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-asynchome-"));
    projectDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-asyncproj-"));
    fs.mkdirSync(path.join(tmpHome, ".agentrace"), { recursive: true });

    received = [];
    server = http.createServer((req, res) => {
      let body = "";
      req.on("data", (c) => (body += c));
      req.on("end", () => {
        received.push(body);
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ ok: true, events_created: 1 }));
      });
    });
    server.on("connection", (s) => {
      sockets.add(s);
      s.on("close", () => sockets.delete(s));
    });
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
    baseUrl = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;

    fs.writeFileSync(
      path.join(tmpHome, ".agentrace", "config.json"),
      JSON.stringify({
        server_url: baseUrl,
        api_key: "agtr_test",
        send_mode: "async",
      })
    );
  });

  afterEach(async () => {
    for (const s of sockets) s.destroy();
    sockets.clear();
    await new Promise<void>((r) => server.close(() => r()));
    fs.rmSync(tmpHome, { recursive: true, force: true });
    fs.rmSync(projectDir, { recursive: true, force: true });
  });

  it("the hook send exits immediately and a detached worker delivers", async () => {
    const transcriptPath = path.join(projectDir, "t.jsonl");
    fs.writeFileSync(transcriptPath, '{"type":"user","uuid":"a1"}\n');

    const exitCode = await new Promise<number>((resolve) => {
      const child = spawn(process.execPath, ["--import", "tsx", indexPath, "send"], {
        env: { ...process.env, HOME: tmpHome, USERPROFILE: tmpHome },
        stdio: ["pipe", "ignore", "ignore"],
      });
      child.stdin!.end(
        JSON.stringify({
          session_id: "async-handoff",
          transcript_path: transcriptPath,
          cwd: projectDir,
        })
      );
      child.on("close", (c) => resolve(c ?? -1));
    });

    // The parent (hook) returns success without waiting for the HTTPS send.
    expect(exitCode).toBe(0);

    // The detached worker delivers shortly after, off the critical path.
    const deadline = Date.now() + 8000;
    while (received.length === 0 && Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 50));
    }

    expect(received).toHaveLength(1);
    expect(JSON.parse(received[0]).session_id).toBe("async-handoff");
  }, 30_000);
});
