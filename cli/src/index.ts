#!/usr/bin/env node

import { Command } from "commander";
import { initCommand } from "./commands/init.js";
import { loginCommand } from "./commands/login.js";
import { sendCommand, sendManualCommand } from "./commands/send.js";
import { uninstallCommand } from "./commands/uninstall.js";
import { onCommand } from "./commands/on.js";
import { offCommand } from "./commands/off.js";
import { mcpServerCommand } from "./commands/mcp-server.js";
import { doctorCommand } from "./commands/doctor.js";
import { workerMain } from "./send/worker.js";

const program = new Command();

program.name("agentrace").description("CLI for AgenTrace").version("0.1.0");

program
  .command("init")
  .description("Initialize agentrace configuration and hooks")
  .requiredOption("--url <url>", "Server URL (required)")
  .option("--proxy <url>", "HTTP/HTTPS proxy URL")
  .option("--dev", "Use local CLI path for development")
  .option("--local", "Install hooks/MCP for current project only (project-local scope)")
  .option("--separate-local-config", "Store config in project directory (requires --local)")
  .option("--async", "Send transcripts asynchronously (off the hook critical path)")
  .action(async (options: { url: string; proxy?: string; dev?: boolean; local?: boolean; separateLocalConfig?: boolean; async?: boolean }) => {
    await initCommand({
      url: options.url,
      proxy: options.proxy,
      dev: options.dev,
      local: options.local,
      separateLocalConfig: options.separateLocalConfig,
      async: options.async,
    });
  });

program
  .command("login")
  .description("Open web dashboard in browser")
  .action(async () => {
    await loginCommand();
  });

program
  .command("send")
  .description("Send event to server (used by hooks, or manually with --claude-session-id)")
  .option("--claude-session-id <id>", "Send existing Claude session by ID")
  .action(async (options: { claudeSessionId?: string }) => {
    if (options.claudeSessionId) {
      await sendManualCommand({ sessionId: options.claudeSessionId });
    } else {
      await sendCommand();
    }
  });

program
  .command("uninstall")
  .description("Remove agentrace hooks and config")
  .option("--local", "Remove only project-local hooks/MCP/config")
  .action(async (options: { local?: boolean }) => {
    await uninstallCommand({ local: options.local });
  });

program
  .command("on")
  .description("Enable agentrace hooks (credentials preserved)")
  .option("--dev", "Use local CLI path for development")
  .option("--local", "Enable hooks/MCP for current project only")
  .option("--async", "Switch send mode to asynchronous")
  .action(async (options: { dev?: boolean; local?: boolean; async?: boolean }) => {
    await onCommand({ dev: options.dev, local: options.local, async: options.async });
  });

program
  .command("off")
  .description("Disable agentrace hooks (credentials preserved)")
  .option("--local", "Disable hooks/MCP for current project only")
  .action(async (options: { local?: boolean }) => {
    await offCommand({ local: options.local });
  });

program
  .command("mcp-server")
  .description("Run MCP server for Claude Code integration (stdio)")
  .action(async () => {
    await mcpServerCommand();
  });

program
  .command("doctor")
  .description("Check configuration and server connectivity")
  .action(async () => {
    await doctorCommand();
  });

program
  .command("__send-worker", { hidden: true })
  .action(async () => {
    await workerMain();
    process.exit(0);
  });

program.parse();
