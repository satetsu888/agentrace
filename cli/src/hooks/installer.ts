import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";

const GLOBAL_CLAUDE_SETTINGS_PATH = path.join(os.homedir(), ".claude", "settings.json");
// MCP servers are configured in ~/.claude.json, NOT in settings.json
const CLAUDE_CONFIG_PATH = path.join(os.homedir(), ".claude.json");
// Agentrace hooks directory
const AGENTRACE_HOOKS_DIR = path.join(os.homedir(), ".agentrace", "hooks");
const SESSION_ID_HOOK_PATH = path.join(AGENTRACE_HOOKS_DIR, "inject-session-id.js");

// Helper to get settings path based on local/global scope
function getSettingsPath(options: { local?: boolean; projectDir?: string }): string {
  if (options.local && options.projectDir) {
    return path.join(options.projectDir, ".claude", "settings.json");
  }
  return GLOBAL_CLAUDE_SETTINGS_PATH;
}

interface ClaudeHook {
  type: string;
  command: string;
}

interface ClaudeHookMatcher {
  matcher?: string;
  hooks: ClaudeHook[];
}

interface McpServerConfig {
  command: string;
  args: string[];
}

interface ClaudeSettings {
  hooks?: {
    Stop?: ClaudeHookMatcher[];
    UserPromptSubmit?: ClaudeHookMatcher[];
    SubagentStop?: ClaudeHookMatcher[];
    PreToolUse?: ClaudeHookMatcher[];
    PostToolUse?: ClaudeHookMatcher[];
    [key: string]: ClaudeHookMatcher[] | undefined;
  };
  [key: string]: unknown;
}

// ~/.claude.json structure (separate from settings.json)
interface ClaudeProjectConfig {
  mcpServers?: {
    [key: string]: McpServerConfig;
  };
  [key: string]: unknown;
}

interface ClaudeConfig {
  mcpServers?: {
    [key: string]: McpServerConfig;
  };
  projects?: {
    [projectPath: string]: ClaudeProjectConfig;
  };
  [key: string]: unknown;
}

const DEFAULT_COMMAND = "npx agentrace send";

function createAgentraceHook(command: string): ClaudeHook {
  return {
    type: "command",
    command,
  };
}

function isAgentraceHook(hook: ClaudeHook): boolean {
  // Match both production ("agentrace send") and dev mode ("index.ts send")
  return hook.command?.includes("agentrace send") || hook.command?.includes("index.ts send");
}

export interface InstallHooksOptions {
  command?: string;
  local?: boolean;
  projectDir?: string;
}

export function installHooks(options: InstallHooksOptions = {}): { success: boolean; message: string } {
  const command = options.command || DEFAULT_COMMAND;
  const agentraceHook = createAgentraceHook(command);
  const settingsPath = getSettingsPath(options);

  try {
    let settings: ClaudeSettings = {};

    // Load existing settings if file exists
    if (fs.existsSync(settingsPath)) {
      const content = fs.readFileSync(settingsPath, "utf-8");
      settings = JSON.parse(content);
    }

    // Initialize hooks structure if not present
    if (!settings.hooks) {
      settings.hooks = {};
    }

    // Add Stop hook (transcript diff is sent on each Stop)
    if (!settings.hooks.Stop) {
      settings.hooks.Stop = [];
    }

    // Add UserPromptSubmit hook (transcript is sent when user sends a message)
    if (!settings.hooks.UserPromptSubmit) {
      settings.hooks.UserPromptSubmit = [];
    }

    // Add SubagentStop hook (transcript is sent when a subagent task completes)
    if (!settings.hooks.SubagentStop) {
      settings.hooks.SubagentStop = [];
    }

    // Add PostToolUse hook (transcript is sent after each tool use for real-time updates)
    if (!settings.hooks.PostToolUse) {
      settings.hooks.PostToolUse = [];
    }

    const hasStopHook = settings.hooks.Stop.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    const hasUserPromptSubmitHook = settings.hooks.UserPromptSubmit.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    const hasSubagentStopHook = settings.hooks.SubagentStop.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    const hasPostToolUseHook = settings.hooks.PostToolUse.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    if (hasStopHook && hasUserPromptSubmitHook && hasSubagentStopHook && hasPostToolUseHook) {
      return { success: true, message: "Hooks already installed (skipped)" };
    }

    if (!hasStopHook) {
      settings.hooks.Stop.push({
        hooks: [agentraceHook],
      });
    }

    if (!hasUserPromptSubmitHook) {
      settings.hooks.UserPromptSubmit.push({
        hooks: [agentraceHook],
      });
    }

    if (!hasSubagentStopHook) {
      settings.hooks.SubagentStop.push({
        hooks: [agentraceHook],
      });
    }

    if (!hasPostToolUseHook) {
      settings.hooks.PostToolUse.push({
        hooks: [agentraceHook],
      });
    }

    // Ensure directory exists
    const dir = path.dirname(settingsPath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }

    // Write settings
    fs.writeFileSync(settingsPath, JSON.stringify(settings, null, 2));

    return { success: true, message: `Hooks added to ${settingsPath}` };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { success: false, message: `Failed to install hooks: ${message}` };
  }
}

export interface UninstallHooksOptions {
  local?: boolean;
  projectDir?: string;
}

export function uninstallHooks(options: UninstallHooksOptions = {}): { success: boolean; message: string } {
  const settingsPath = getSettingsPath(options);

  try {
    if (!fs.existsSync(settingsPath)) {
      return { success: true, message: "No settings file found" };
    }

    const content = fs.readFileSync(settingsPath, "utf-8");
    const settings: ClaudeSettings = JSON.parse(content);

    if (!settings.hooks) {
      return { success: true, message: "No hooks configured" };
    }

    // Remove agentrace hooks from Stop
    if (settings.hooks.Stop) {
      settings.hooks.Stop = settings.hooks.Stop.filter(
        (matcher) => !matcher.hooks?.some(isAgentraceHook)
      );
      if (settings.hooks.Stop.length === 0) {
        delete settings.hooks.Stop;
      }
    }

    // Remove agentrace hooks from UserPromptSubmit
    if (settings.hooks.UserPromptSubmit) {
      settings.hooks.UserPromptSubmit = settings.hooks.UserPromptSubmit.filter(
        (matcher) => !matcher.hooks?.some(isAgentraceHook)
      );
      if (settings.hooks.UserPromptSubmit.length === 0) {
        delete settings.hooks.UserPromptSubmit;
      }
    }

    // Remove agentrace hooks from SubagentStop
    if (settings.hooks.SubagentStop) {
      settings.hooks.SubagentStop = settings.hooks.SubagentStop.filter(
        (matcher) => !matcher.hooks?.some(isAgentraceHook)
      );
      if (settings.hooks.SubagentStop.length === 0) {
        delete settings.hooks.SubagentStop;
      }
    }

    // Remove agentrace hooks from PostToolUse
    if (settings.hooks.PostToolUse) {
      settings.hooks.PostToolUse = settings.hooks.PostToolUse.filter(
        (matcher) => !matcher.hooks?.some(isAgentraceHook)
      );
      if (settings.hooks.PostToolUse.length === 0) {
        delete settings.hooks.PostToolUse;
      }
    }

    // Clean up empty hooks object
    if (Object.keys(settings.hooks).length === 0) {
      delete settings.hooks;
    }

    fs.writeFileSync(settingsPath, JSON.stringify(settings, null, 2));

    return {
      success: true,
      message: `Removed hooks from ${settingsPath}`,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { success: false, message: `Failed to uninstall hooks: ${message}` };
  }
}

export interface CheckHooksOptions {
  local?: boolean;
  projectDir?: string;
}

export function checkHooksInstalled(options: CheckHooksOptions = {}): boolean {
  const settingsPath = getSettingsPath(options);
  try {
    if (!fs.existsSync(settingsPath)) {
      return false;
    }

    const content = fs.readFileSync(settingsPath, "utf-8");
    const settings: ClaudeSettings = JSON.parse(content);

    const hasStopHook = settings.hooks?.Stop?.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    const hasUserPromptSubmitHook = settings.hooks?.UserPromptSubmit?.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    const hasSubagentStopHook = settings.hooks?.SubagentStop?.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    const hasPostToolUseHook = settings.hooks?.PostToolUse?.some((matcher) =>
      matcher.hooks?.some(isAgentraceHook)
    );

    return !!hasStopHook && !!hasUserPromptSubmitHook && !!hasSubagentStopHook && !!hasPostToolUseHook;
  } catch {
    return false;
  }
}

// MCP Server installer functions

const MCP_SERVER_NAME = "agentrace";

export interface InstallMcpServerOptions {
  command?: string;
  args?: string[];
  local?: boolean;
  projectDir?: string;
}

export function installMcpServer(options: InstallMcpServerOptions = {}): { success: boolean; message: string } {
  const command = options.command || "npx";
  const args = options.args || ["agentrace", "mcp-server"];

  try {
    let config: ClaudeConfig = {};

    // Load existing config if file exists
    // MCP servers are configured in ~/.claude.json (NOT settings.json)
    if (fs.existsSync(CLAUDE_CONFIG_PATH)) {
      const content = fs.readFileSync(CLAUDE_CONFIG_PATH, "utf-8");
      config = JSON.parse(content);
    }

    if (options.local && options.projectDir) {
      // Local scope: add to projects.{projectDir}.mcpServers
      if (!config.projects) {
        config.projects = {};
      }
      if (!config.projects[options.projectDir]) {
        config.projects[options.projectDir] = {};
      }
      if (!config.projects[options.projectDir].mcpServers) {
        config.projects[options.projectDir].mcpServers = {};
      }

      const projectMcpServers = config.projects[options.projectDir].mcpServers!;
      const alreadyExists = !!projectMcpServers[MCP_SERVER_NAME];

      projectMcpServers[MCP_SERVER_NAME] = { command, args };
      fs.writeFileSync(CLAUDE_CONFIG_PATH, JSON.stringify(config, null, 2));

      return {
        success: true,
        message: alreadyExists
          ? "MCP server config updated (local)"
          : `MCP server added to ${CLAUDE_CONFIG_PATH} (local scope for ${options.projectDir})`,
      };
    } else {
      // User scope: add to mcpServers (global)
      if (!config.mcpServers) {
        config.mcpServers = {};
      }

      const alreadyExists = !!config.mcpServers[MCP_SERVER_NAME];

      config.mcpServers[MCP_SERVER_NAME] = { command, args };
      fs.writeFileSync(CLAUDE_CONFIG_PATH, JSON.stringify(config, null, 2));

      return {
        success: true,
        message: alreadyExists
          ? "MCP server config updated"
          : `MCP server added to ${CLAUDE_CONFIG_PATH}`,
      };
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { success: false, message: `Failed to install MCP server: ${message}` };
  }
}

export interface UninstallMcpServerOptions {
  local?: boolean;
  projectDir?: string;
}

export function uninstallMcpServer(options: UninstallMcpServerOptions = {}): { success: boolean; message: string } {
  try {
    if (!fs.existsSync(CLAUDE_CONFIG_PATH)) {
      return { success: true, message: "No config file found" };
    }

    const content = fs.readFileSync(CLAUDE_CONFIG_PATH, "utf-8");
    const config: ClaudeConfig = JSON.parse(content);

    if (options.local && options.projectDir) {
      // Local scope: remove from projects.{projectDir}.mcpServers
      if (!config.projects?.[options.projectDir]?.mcpServers?.[MCP_SERVER_NAME]) {
        return { success: true, message: "MCP server not configured (local)" };
      }

      delete config.projects[options.projectDir].mcpServers![MCP_SERVER_NAME];

      // Clean up empty mcpServers object
      if (Object.keys(config.projects[options.projectDir].mcpServers!).length === 0) {
        delete config.projects[options.projectDir].mcpServers;
      }

      fs.writeFileSync(CLAUDE_CONFIG_PATH, JSON.stringify(config, null, 2));

      return {
        success: true,
        message: `Removed MCP server from ${CLAUDE_CONFIG_PATH} (local scope)`,
      };
    } else {
      // User scope: remove from mcpServers (global)
      if (!config.mcpServers || !config.mcpServers[MCP_SERVER_NAME]) {
        return { success: true, message: "MCP server not configured" };
      }

      delete config.mcpServers[MCP_SERVER_NAME];

      // Clean up empty mcpServers object
      if (Object.keys(config.mcpServers).length === 0) {
        delete config.mcpServers;
      }

      fs.writeFileSync(CLAUDE_CONFIG_PATH, JSON.stringify(config, null, 2));

      return {
        success: true,
        message: `Removed MCP server from ${CLAUDE_CONFIG_PATH}`,
      };
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { success: false, message: `Failed to uninstall MCP server: ${message}` };
  }
}

export interface CheckMcpServerOptions {
  local?: boolean;
  projectDir?: string;
}

export function checkMcpServerInstalled(options: CheckMcpServerOptions = {}): boolean {
  try {
    if (!fs.existsSync(CLAUDE_CONFIG_PATH)) {
      return false;
    }

    const content = fs.readFileSync(CLAUDE_CONFIG_PATH, "utf-8");
    const config: ClaudeConfig = JSON.parse(content);

    if (options.local && options.projectDir) {
      return !!config.projects?.[options.projectDir]?.mcpServers?.[MCP_SERVER_NAME];
    } else {
      return !!config.mcpServers?.[MCP_SERVER_NAME];
    }
  } catch {
    return false;
  }
}

// PreToolUse hook for injecting session_id into agentrace MCP tools

const SESSION_ID_HOOK_SCRIPT = `#!/usr/bin/env node
// Agentrace PreToolUse hook: Writes session_id and tool_use_id to file for MCP tools
// This hook is called before agentrace MCP tools (create_plan, update_plan)

const fs = require('fs');
const os = require('os');
const path = require('path');

// CLAUDE_PROJECT_DIR を優先（MCP サーバーコンテキスト対応）
// local config が存在する場合はプロジェクトディレクトリを使用
const projectDir = process.env.CLAUDE_PROJECT_DIR || process.cwd();
const localSessionFile = path.join(projectDir, '.agentrace', 'current-session.json');
const globalSessionFile = path.join(os.homedir(), '.agentrace', 'current-session.json');

// local config が存在するかチェックしてセッションファイルの場所を決定
const localConfigExists = fs.existsSync(path.join(projectDir, '.agentrace', 'config.json'));
const sessionFile = localConfigExists ? localSessionFile : globalSessionFile;

let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => { input += chunk; });
process.stdin.on('end', () => {
  try {
    const data = JSON.parse(input);
    const sessionId = data.session_id;
    const toolUseId = data.tool_use_id;

    // Write session_id and tool_use_id to file for MCP server to read
    fs.writeFileSync(sessionFile, JSON.stringify({ session_id: sessionId, tool_use_id: toolUseId }));

    // Allow the tool to proceed
    const output = {
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        permissionDecision: "allow"
      }
    };
    console.log(JSON.stringify(output));
  } catch (e) {
    process.stderr.write('Error: ' + e.message);
    process.exit(1);
  }
});
`;

const AGENTRACE_MCP_TOOLS_MATCHER = "mcp__agentrace__create_plan|mcp__agentrace__update_plan";

function isAgentracePreToolUseHook(matcher: ClaudeHookMatcher): boolean {
  return matcher.matcher === AGENTRACE_MCP_TOOLS_MATCHER &&
    matcher.hooks?.some(h => h.command?.includes("inject-session-id"));
}

export interface InstallPreToolUseHookOptions {
  local?: boolean;
  projectDir?: string;
}

export function installPreToolUseHook(options: InstallPreToolUseHookOptions = {}): { success: boolean; message: string } {
  const settingsPath = getSettingsPath(options);

  try {
    // Create hooks directory if not exists
    if (!fs.existsSync(AGENTRACE_HOOKS_DIR)) {
      fs.mkdirSync(AGENTRACE_HOOKS_DIR, { recursive: true });
    }

    // Write hook script
    fs.writeFileSync(SESSION_ID_HOOK_PATH, SESSION_ID_HOOK_SCRIPT, { mode: 0o755 });

    // Load existing settings
    let settings: ClaudeSettings = {};
    if (fs.existsSync(settingsPath)) {
      const content = fs.readFileSync(settingsPath, "utf-8");
      settings = JSON.parse(content);
    }

    // Initialize hooks structure if not present
    if (!settings.hooks) {
      settings.hooks = {};
    }
    if (!settings.hooks.PreToolUse) {
      settings.hooks.PreToolUse = [];
    }

    // Check if already installed
    const hasPreToolUseHook = settings.hooks.PreToolUse.some(isAgentracePreToolUseHook);
    if (hasPreToolUseHook) {
      return { success: true, message: "PreToolUse hook already installed (skipped)" };
    }

    // Add PreToolUse hook
    settings.hooks.PreToolUse.push({
      matcher: AGENTRACE_MCP_TOOLS_MATCHER,
      hooks: [
        {
          type: "command",
          command: SESSION_ID_HOOK_PATH,
        },
      ],
    });

    // Ensure directory exists
    const dir = path.dirname(settingsPath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }

    // Write settings
    fs.writeFileSync(settingsPath, JSON.stringify(settings, null, 2));

    return { success: true, message: `PreToolUse hook installed to ${SESSION_ID_HOOK_PATH}` };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { success: false, message: `Failed to install PreToolUse hook: ${message}` };
  }
}

export interface UninstallPreToolUseHookOptions {
  local?: boolean;
  projectDir?: string;
  // If true, also remove the hook script file (only when uninstalling global hooks)
  removeScript?: boolean;
}

export function uninstallPreToolUseHook(options: UninstallPreToolUseHookOptions = {}): { success: boolean; message: string } {
  const settingsPath = getSettingsPath(options);

  try {
    // Remove hook script only when uninstalling global hooks (not local)
    if (!options.local && options.removeScript !== false && fs.existsSync(SESSION_ID_HOOK_PATH)) {
      fs.unlinkSync(SESSION_ID_HOOK_PATH);
    }

    // Remove from settings
    if (!fs.existsSync(settingsPath)) {
      return { success: true, message: "No settings file found" };
    }

    const content = fs.readFileSync(settingsPath, "utf-8");
    const settings: ClaudeSettings = JSON.parse(content);

    if (!settings.hooks?.PreToolUse) {
      return { success: true, message: "No PreToolUse hooks configured" };
    }

    // Remove agentrace PreToolUse hooks
    settings.hooks.PreToolUse = settings.hooks.PreToolUse.filter(
      (matcher) => !isAgentracePreToolUseHook(matcher)
    );

    if (settings.hooks.PreToolUse.length === 0) {
      delete settings.hooks.PreToolUse;
    }

    // Clean up empty hooks object
    if (Object.keys(settings.hooks).length === 0) {
      delete settings.hooks;
    }

    fs.writeFileSync(settingsPath, JSON.stringify(settings, null, 2));

    return { success: true, message: "PreToolUse hook removed" };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { success: false, message: `Failed to uninstall PreToolUse hook: ${message}` };
  }
}

export interface CheckPreToolUseHookOptions {
  local?: boolean;
  projectDir?: string;
}

export function checkPreToolUseHookInstalled(options: CheckPreToolUseHookOptions = {}): boolean {
  const settingsPath = getSettingsPath(options);

  try {
    if (!fs.existsSync(settingsPath) || !fs.existsSync(SESSION_ID_HOOK_PATH)) {
      return false;
    }

    const content = fs.readFileSync(settingsPath, "utf-8");
    const settings: ClaudeSettings = JSON.parse(content);

    return settings.hooks?.PreToolUse?.some(isAgentracePreToolUseHook) ?? false;
  } catch {
    return false;
  }
}
