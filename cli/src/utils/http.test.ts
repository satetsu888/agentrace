import { describe, it, expect, beforeEach, afterEach } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import * as http from "node:http";
import { sendIngest, SEND_TIMEOUT_MS } from "./http.js";
import { saveLocalConfig } from "../config/manager.js";

describe("utils/http sendIngest", () => {
  let tempProjectDir: string;
  let server: http.Server;
  let baseUrl: string;
  const sockets = new Set<import("node:net").Socket>();
  // Controls whether the test server ever responds.
  let hang = false;

  beforeEach(async () => {
    tempProjectDir = fs.mkdtempSync(path.join(os.tmpdir(), "agentrace-http-"));

    server = http.createServer((_req, res) => {
      if (hang) {
        // Never respond — forces the client-side timeout to fire.
        return;
      }
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ ok: true, events_created: 1 }));
    });
    server.on("connection", (socket) => {
      sockets.add(socket);
      socket.on("close", () => sockets.delete(socket));
    });

    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const addr = server.address();
    if (addr && typeof addr === "object") {
      baseUrl = `http://127.0.0.1:${addr.port}`;
    }

    saveLocalConfig(tempProjectDir, {
      server_url: baseUrl,
      api_key: "agtr_test",
    });
  });

  afterEach(async () => {
    delete process.env.AGENTRACE_SEND_TIMEOUT_MS;
    hang = false;
    for (const socket of sockets) socket.destroy();
    sockets.clear();
    await new Promise<void>((resolve) => server.close(() => resolve()));
    if (fs.existsSync(tempProjectDir)) {
      fs.rmSync(tempProjectDir, { recursive: true });
    }
  });

  it("exposes a tunable default send timeout longer than a request", () => {
    expect(SEND_TIMEOUT_MS).toBeGreaterThanOrEqual(30_000);
  });

  it("returns ok on a successful response", async () => {
    const result = await sendIngest(
      { session_id: "s1", transcript_lines: [{ type: "user" }] },
      tempProjectDir
    );
    expect(result.ok).toBe(true);
  });

  it("returns { ok: false } when the request exceeds the send timeout", async () => {
    hang = true;
    process.env.AGENTRACE_SEND_TIMEOUT_MS = "150";

    const result = await sendIngest(
      { session_id: "s1", transcript_lines: [{ type: "user" }] },
      tempProjectDir
    );

    expect(result.ok).toBe(false);
    expect(result.error).toBeTruthy();
  });
});
