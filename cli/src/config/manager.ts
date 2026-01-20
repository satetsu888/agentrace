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
