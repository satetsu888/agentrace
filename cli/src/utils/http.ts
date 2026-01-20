import { ProxyAgent } from "undici";
import { loadConfig, type AgentraceConfig } from "../config/manager.js";

/**
 * プロキシURLを解決する。
 * 優先順位: 設定ファイル > HTTPS_PROXY > HTTP_PROXY（小文字も対応）
 */
function resolveProxyUrl(config: AgentraceConfig | null): string | undefined {
  if (config?.proxy_url) {
    return config.proxy_url;
  }
  return (
    process.env.HTTPS_PROXY ||
    process.env.https_proxy ||
    process.env.HTTP_PROXY ||
    process.env.http_proxy
  );
}

/**
 * NO_PROXYに該当するかどうかを判定する。
 */
function shouldBypassProxy(targetUrl: string): boolean {
  const noProxy = process.env.NO_PROXY || process.env.no_proxy;
  if (!noProxy) {
    return false;
  }

  const hostname = new URL(targetUrl).hostname;
  const entries = noProxy.split(",").map((e) => e.trim().toLowerCase());

  for (const entry of entries) {
    if (entry === "*") {
      return true;
    }
    const hostLower = hostname.toLowerCase();
    if (entry.startsWith(".")) {
      if (hostLower.endsWith(entry) || hostLower === entry.slice(1)) {
        return true;
      }
    } else if (hostLower === entry || hostLower.endsWith(`.${entry}`)) {
      return true;
    }
  }
  return false;
}

/**
 * プロキシ対応のfetchを実行する。
 */
async function fetchWithProxy(
  url: string,
  options: RequestInit,
  config: AgentraceConfig | null
): Promise<Response> {
  const proxyUrl = resolveProxyUrl(config);
  if (proxyUrl && !shouldBypassProxy(url)) {
    const agent = new ProxyAgent(proxyUrl);
    return fetch(url, {
      ...options,
      dispatcher: agent,
    } as RequestInit);
  }
  return fetch(url, options);
}

export interface IngestPayload {
  session_id: string;
  transcript_lines: unknown[];
  cwd?: string;
  git_remote_url?: string;
  git_branch?: string;
  // Subagent (Task tool) fields
  parent_session_id?: string;
  agent_id?: string;
  is_sidechain?: boolean;
  title?: string;
}

export interface IngestResponse {
  ok: boolean;
  events_created?: number;
  error?: string;
}

export interface WebSessionResponse {
  url: string;
  expires_at: string;
}

function getBaseUrl(config: { server_url: string }): string {
  return config.server_url.replace(/\/+$/, '');
}

export async function sendIngest(
  payload: IngestPayload
): Promise<IngestResponse> {
  const config = loadConfig();
  if (!config) {
    return { ok: false, error: "Config not found" };
  }

  const url = `${getBaseUrl(config)}/api/ingest`;

  try {
    const response = await fetchWithProxy(
      url,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${config.api_key}`,
        },
        body: JSON.stringify(payload),
      },
      config
    );

    if (!response.ok) {
      const text = await response.text();
      return { ok: false, error: `HTTP ${response.status}: ${text}` };
    }

    return (await response.json()) as IngestResponse;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { ok: false, error: message };
  }
}

export async function createWebSession(): Promise<
  { ok: true; data: WebSessionResponse } | { ok: false; error: string }
> {
  const config = loadConfig();
  if (!config) {
    return { ok: false, error: "Config not found. Run 'agentrace init' first." };
  }

  const url = `${getBaseUrl(config)}/api/auth/web-session`;

  try {
    const response = await fetchWithProxy(
      url,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${config.api_key}`,
        },
      },
      config
    );

    if (!response.ok) {
      const text = await response.text();
      return { ok: false, error: `HTTP ${response.status}: ${text}` };
    }

    const data = (await response.json()) as WebSessionResponse;
    return { ok: true, data };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { ok: false, error: message };
  }
}
