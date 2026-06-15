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

describe("__send-worker entry", () => {
  let tmpHome: string;
  let projectDir: string;
  let server: http.Server;
  let baseUrl: string;
  let received: string[];
  const sockets = new Set<import("node:net").Socket>();

  beforeEach(async () => {
    tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-entryhome-"));
    projectDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-entryproj-"));
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
      JSON.stringify({ server_url: baseUrl, api_key: "agtr_test" })
    );
  });

  afterEach(async () => {
    for (const s of sockets) s.destroy();
    sockets.clear();
    await new Promise<void>((r) => server.close(() => r()));
    fs.rmSync(tmpHome, { recursive: true, force: true });
    fs.rmSync(projectDir, { recursive: true, force: true });
  });

  it("delivers the transcript via the worker env contract and exits 0", async () => {
    const transcriptPath = path.join(projectDir, "t.jsonl");
    fs.writeFileSync(
      transcriptPath,
      '{"type":"user","uuid":"e1"}\n{"type":"assistant","uuid":"e2"}\n'
    );

    const code = await new Promise<number>((resolve) => {
      const child = spawn(
        process.execPath,
        ["--import", "tsx", indexPath, "__send-worker"],
        {
          env: {
            ...process.env,
            HOME: tmpHome,
            USERPROFILE: tmpHome,
            AGENTRACE_WORKER_SESSION_ID: "entry-sess",
            AGENTRACE_WORKER_TRANSCRIPT_PATH: transcriptPath,
            AGENTRACE_WORKER_PROJECT_DIR: projectDir,
          },
          stdio: "ignore",
        }
      );
      child.on("close", (c) => resolve(c ?? -1));
    });

    expect(code).toBe(0);
    expect(received).toHaveLength(1);
    expect(JSON.parse(received[0]).transcript_lines).toHaveLength(2);
  }, 30_000);
});
