import * as path from "node:path";
import { fileURLToPath } from "node:url";
import { ProxyAgent } from "undici";
import { saveConfig, getConfigPath, saveLocalConfig, getLocalConfigPath } from "../config/manager.js";
import { installHooks, installMcpServer, installPreToolUseHook } from "../hooks/installer.js";
import {
  startCallbackServer,
  getRandomPort,
  generateToken,
} from "../utils/callback-server.js";
import { openBrowser, buildSetupUrl } from "../utils/browser.js";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const CALLBACK_TIMEOUT = 5 * 60 * 1000; // 5 minutes

export interface InitOptions {
  url?: string;
  proxy?: string;
  dev?: boolean;
  local?: boolean;
  separateLocalConfig?: boolean;
  async?: boolean;
}

export async function initCommand(options: InitOptions = {}): Promise<void> {
  // --url is required
  if (!options.url) {
    console.error("Error: --url option is required");
    console.error("");
    console.error("Usage: npx agentrace init --url <server-url>");
    console.error("Example: npx agentrace init --url http://localhost:8080");
    process.exit(1);
  }

  // Validate URL
  let serverUrl: URL;
  try {
    serverUrl = new URL(options.url);
  } catch {
    console.error("Error: Invalid URL format");
    process.exit(1);
  }

  // Validate proxy URL if provided (using ProxyAgent to match actual usage)
  if (options.proxy) {
    try {
      new ProxyAgent(options.proxy);
    } catch (error) {
      console.error("Error: Invalid proxy URL format");
      console.error(`  ${error instanceof Error ? error.message : String(error)}`);
      console.error("");
      console.error("Example: http://proxy.example.com:8080");
      console.error("         http://user:pass@proxy.example.com:8080");
      process.exit(1);
    }
  }

  console.log("AgenTrace Setup\n");

  if (options.dev) {
    console.log("[Dev Mode] Using local CLI for hooks\n");
  }

  if (options.local) {
    console.log("[Local Mode] Installing hooks/MCP for this project only\n");
  }

  // Project directory for local scope
  const projectDir = options.local ? process.cwd() : undefined;

  // Generate token and start callback server
  const token = generateToken();
  const port = getRandomPort();
  const callbackUrl = `http://127.0.0.1:${port}/callback`;

  console.log("Starting local callback server...");

  // Start callback server (returns promise that resolves when callback is received)
  const callbackPromise = startCallbackServer(port, {
    token,
    timeout: CALLBACK_TIMEOUT,
  });

  // Build setup URL and open browser
  const setupUrl = buildSetupUrl(serverUrl.toString(), token, callbackUrl);

  console.log(`Opening browser for authentication...`);
  const browserResult = await openBrowser(setupUrl);

  if (!browserResult.success) {
    console.log("");
    console.log("Could not open browser automatically.");
    console.log("Please open this URL manually:");
    console.log("");
    console.log(`  ${setupUrl}`);
    console.log("");
  }

  console.log("Waiting for setup to complete...");
  console.log("(This will timeout in 5 minutes)\n");

  try {
    // Wait for callback
    const result = await callbackPromise;

    // Save config (remove trailing slash from URL)
    const serverUrlStr = serverUrl.toString().replace(/\/+$/, '');
    const configData = {
      server_url: serverUrlStr,
      api_key: result.apiKey,
      ...(options.proxy && { proxy_url: options.proxy }),
      ...(options.async && { send_mode: "async" as const }),
    };

    if (options.local && options.separateLocalConfig && projectDir) {
      // Save config locally in project directory
      saveLocalConfig(projectDir, configData);
      console.log(`✓ Config saved to ${getLocalConfigPath(projectDir)}`);
      console.log(`⚠ Remember to add '.agentrace/' to your .gitignore`);
    } else {
      // Save config globally
      saveConfig(configData);
      console.log(`✓ Config saved to ${getConfigPath()}`);
    }
    if (options.proxy) {
      console.log(`  Proxy: ${options.proxy}`);
    }
    if (options.async) {
      console.log(`  Send mode: async`);
    }

    // Determine hook command
    let hookCommand: string | undefined;
    if (options.dev) {
      // Use local CLI path for development
      const cliRoot = path.resolve(__dirname, "../..");
      const indexPath = path.join(cliRoot, "src/index.ts");
      hookCommand = `npx tsx ${indexPath} send`;
      console.log(`  Hook command: ${hookCommand}`);
    }

    // Install hooks
    const hookResult = installHooks({
      command: hookCommand,
      local: options.local,
      projectDir,
    });
    if (hookResult.success) {
      console.log(`✓ ${hookResult.message}`);
    } else {
      console.error(`✗ ${hookResult.message}`);
    }

    // Install MCP server
    let mcpCommand: string | undefined;
    let mcpArgs: string[] | undefined;
    if (options.dev) {
      const cliRoot = path.resolve(__dirname, "../..");
      const indexPath = path.join(cliRoot, "src/index.ts");
      mcpCommand = "npx";
      mcpArgs = ["tsx", indexPath, "mcp-server"];
    }
    const mcpResult = installMcpServer({
      command: mcpCommand,
      args: mcpArgs,
      local: options.local,
      projectDir,
    });
    if (mcpResult.success) {
      console.log(`✓ ${mcpResult.message}`);
    } else {
      console.error(`✗ ${mcpResult.message}`);
    }

    // Install PreToolUse hook for session_id injection
    const preToolUseResult = installPreToolUseHook({
      local: options.local,
      projectDir,
    });
    if (preToolUseResult.success) {
      console.log(`✓ ${preToolUseResult.message}`);
    } else {
      console.error(`✗ ${preToolUseResult.message}`);
    }

    console.log("\n✓ Setup complete!");
  } catch (error) {
    if (error instanceof Error && error.message.includes("Timeout")) {
      console.error("\n✗ Setup timed out.");
      console.error("Please try again with: npx agentrace init --url " + options.url);
    } else {
      console.error("\n✗ Setup failed:", error instanceof Error ? error.message : error);
    }
    process.exit(1);
  }
}
