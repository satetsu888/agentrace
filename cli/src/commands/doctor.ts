import {
  loadConfig,
  loadConfigWithFallback,
  getConfigPath,
  findAndLoadLocalConfig,
  getSendMode,
} from "../config/manager.js";
import { createDispatcher } from "../utils/proxy.js";
import { fetch } from "undici";

interface VersionResponse {
  version: string;
}

export async function doctorCommand(): Promise<void> {
  const cwd = process.cwd();

  console.log("AgenTrace Doctor\n");
  console.log("================\n");

  // CLI Version (fallback for tsx dev mode)
  const version = typeof __CLI_VERSION__ !== "undefined" ? __CLI_VERSION__ : "dev";
  console.log(`CLI Version: ${version}`);
  console.log("");

  // Check config locations
  const globalConfigPath = getConfigPath();
  const globalConfig = loadConfig();
  const localResult = findAndLoadLocalConfig(cwd);
  const effectiveConfig = loadConfigWithFallback(cwd);

  console.log("Configuration:");
  console.log(`  Global config: ${globalConfigPath}`);
  console.log(`    Status: ${globalConfig ? "✓ Found" : "✗ Not found"}`);
  console.log("");

  if (localResult) {
    console.log(`  Local config: ${localResult.path}`);
    console.log(`    Status: ✓ Found`);
  } else {
    console.log(`  Local config: (none found in parent directories)`);
    console.log(`    Status: ✗ Not found`);
  }
  console.log("");

  if (effectiveConfig) {
    const configSource = localResult ? "Local" : "Global";
    const configPath = localResult ? localResult.path : globalConfigPath;
    console.log(`  Active config: ${configSource} (${configPath})`);
    console.log(`  Server URL: ${effectiveConfig.server_url}`);
    console.log(`  API Key: ${maskApiKey(effectiveConfig.api_key)}`);
    console.log(`  Send mode: ${getSendMode(effectiveConfig)}`);
    if (effectiveConfig.proxy_url) {
      console.log(`  Proxy URL: ${effectiveConfig.proxy_url}`);
    }
  } else {
    console.log("  Active config: None");
    console.log("");
    console.log("Run 'npx agentrace init --url <server-url>' to configure.");
    return;
  }

  console.log("");

  // Server connectivity
  console.log("Server Status:");
  try {
    const baseUrl = effectiveConfig.server_url.replace(/\/+$/, "");
    const response = await fetch(`${baseUrl}/api/version`, {
      method: "GET",
      headers: {
        "Content-Type": "application/json",
      },
      dispatcher: createDispatcher(cwd),
    });

    if (response.ok) {
      const data = (await response.json()) as VersionResponse;
      console.log(`  Connection: ✓ Connected`);
      console.log(`  Server Version: ${data.version}`);
    } else {
      console.log(`  Connection: ✗ Error (HTTP ${response.status})`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.log(`  Connection: ✗ Failed`);
    console.log(`  Error: ${message}`);
  }
}

function maskApiKey(apiKey: string): string {
  if (!apiKey || apiKey.length < 12) {
    return "****";
  }
  // Show first 8 chars (e.g., "agtr_xxx") and mask the rest
  return apiKey.slice(0, 8) + "****" + apiKey.slice(-4);
}
