import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";

export interface AgentraceConfig {
  server_url: string;
  api_key: string;
  proxy_url?: string;
}

const CONFIG_DIR = path.join(os.homedir(), ".agentrace");
const CONFIG_FILE = path.join(CONFIG_DIR, "config.json");

export function getConfigPath(): string {
  return CONFIG_FILE;
}

export function loadConfig(): AgentraceConfig | null {
  try {
    if (!fs.existsSync(CONFIG_FILE)) {
      return null;
    }
    const content = fs.readFileSync(CONFIG_FILE, "utf-8");
    return JSON.parse(content) as AgentraceConfig;
  } catch {
    return null;
  }
}

export function saveConfig(config: AgentraceConfig): void {
  if (!fs.existsSync(CONFIG_DIR)) {
    fs.mkdirSync(CONFIG_DIR, { recursive: true });
  }
  fs.writeFileSync(CONFIG_FILE, JSON.stringify(config, null, 2));
}

export function deleteConfig(): boolean {
  try {
    if (fs.existsSync(CONFIG_FILE)) {
      fs.unlinkSync(CONFIG_FILE);
      return true;
    }
    return false;
  } catch {
    return false;
  }
}

// --- Local (project-level) config functions ---

export function getLocalConfigDir(projectDir: string): string {
  return path.join(projectDir, ".agentrace");
}

export function getLocalConfigPath(projectDir: string): string {
  return path.join(getLocalConfigDir(projectDir), "config.json");
}

export function loadLocalConfig(projectDir: string): AgentraceConfig | null {
  const configFile = getLocalConfigPath(projectDir);
  try {
    if (!fs.existsSync(configFile)) {
      return null;
    }
    const content = fs.readFileSync(configFile, "utf-8");
    return JSON.parse(content) as AgentraceConfig;
  } catch {
    return null;
  }
}

export function saveLocalConfig(projectDir: string, config: AgentraceConfig): void {
  const localConfigDir = getLocalConfigDir(projectDir);
  const localConfigFile = getLocalConfigPath(projectDir);
  if (!fs.existsSync(localConfigDir)) {
    fs.mkdirSync(localConfigDir, { recursive: true });
  }
  fs.writeFileSync(localConfigFile, JSON.stringify(config, null, 2));
}

export function deleteLocalConfig(projectDir: string): boolean {
  const localConfigDir = getLocalConfigDir(projectDir);
  try {
    if (fs.existsSync(localConfigDir)) {
      fs.rmSync(localConfigDir, { recursive: true });
      return true;
    }
    return false;
  } catch {
    return false;
  }
}

/**
 * Load config with fallback: local config > global config
 * Uses CLAUDE_PROJECT_DIR environment variable if projectDir is not specified
 * (supports MCP server context where process.cwd() may not be the project directory)
 */
export function loadConfigWithFallback(projectDir?: string): AgentraceConfig | null {
  // CLAUDE_PROJECT_DIR を優先（MCP サーバーコンテキスト対応）
  const effectiveProjectDir = projectDir || process.env.CLAUDE_PROJECT_DIR || process.cwd();

  const localConfig = loadLocalConfig(effectiveProjectDir);
  if (localConfig) return localConfig;

  return loadConfig();
}
