import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";

export type SendMode = "sync" | "async";

export interface AgentraceConfig {
  server_url: string;
  api_key: string;
  proxy_url?: string;
  send_mode?: SendMode;
}

/**
 * Resolve the effective send mode from a config.
 * Defaults to "sync" when unset, null, or set to any unrecognized value
 * (opt-in / backward-compatible default — HC-1).
 */
export function getSendMode(config: AgentraceConfig | null | undefined): SendMode {
  return config?.send_mode === "async" ? "async" : "sync";
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
 * Find local config by searching up the directory tree from startDir to home directory.
 * Returns the path to the found config, or null if not found.
 */
export function findLocalConfigPath(startDir: string): string | null {
  const homeDir = os.homedir();
  let currentDir = path.resolve(startDir);

  while (currentDir !== homeDir && currentDir !== path.dirname(currentDir)) {
    const configPath = getLocalConfigPath(currentDir);
    if (fs.existsSync(configPath)) {
      return configPath;
    }
    currentDir = path.dirname(currentDir);
  }

  return null;
}

/**
 * Load local config by searching up the directory tree from startDir.
 * Returns the config and its path, or null if not found.
 */
export function findAndLoadLocalConfig(startDir: string): { config: AgentraceConfig; path: string } | null {
  const configPath = findLocalConfigPath(startDir);
  if (!configPath) {
    return null;
  }

  try {
    const content = fs.readFileSync(configPath, "utf-8");
    return {
      config: JSON.parse(content) as AgentraceConfig,
      path: configPath,
    };
  } catch {
    return null;
  }
}

/**
 * Load config with fallback: local config (searching up directory tree) > global config
 */
export function loadConfigWithFallback(projectDir?: string): AgentraceConfig | null {
  if (projectDir) {
    const localResult = findAndLoadLocalConfig(projectDir);
    if (localResult) return localResult.config;
  }
  return loadConfig();
}
