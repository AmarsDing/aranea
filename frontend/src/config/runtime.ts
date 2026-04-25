type RuntimeConfig = {
  /** REST API 根，如 `http://127.0.0.1:8080`；为空则用默认 */
  backendUrl?: string;
  /**
   * ADK HTTP 流式接口所在 **Origin**（无路径），如 `http://127.0.0.1:8000`。
   * 用于 `POST {origin}/run_sse`，与 `/api/v1` 可指向不同服务。
   */
  adkStreamOrigin?: string;
  /**
   * WebSocket 主机，格式 `host:port`，如 `127.0.0.1:8000`。
   * 用于 `ws(s)://{host}/run_live?...`
   */
  adkWsHost?: string;
};

let runtimeConfig: RuntimeConfig = {};

export async function loadRuntimeConfig(): Promise<void> {
  try {
    const resp = await fetch("/assets/config/runtime-config.json", { cache: "no-store" });
    if (!resp.ok) return;
    runtimeConfig = await resp.json();
  } catch {
    runtimeConfig = {};
  }
}

export function getBackendBaseURL(): string {
  if (runtimeConfig.backendUrl && runtimeConfig.backendUrl.trim().length > 0) {
    return `${runtimeConfig.backendUrl.replace(/\/$/, "")}/api/v1`;
  }
  // Vite dev: use same-origin path so the dev server can proxy to the Go API.
  if (import.meta.env.DEV) {
    return "/api/v1";
  }
  return "http://localhost:8080/api/v1";
}

/**
 * 用于 `POST .../run_sse` 的绝对 URL。
 * 未配置 `adkStreamOrigin` 时：开发环境走同源相对路径 `/run_sse`（由 devServer 代理到后端/ADK）；生产为当前站点 `origin`。
 */
export function getAdkRunSseUrl(): string {
  if (runtimeConfig.adkStreamOrigin && runtimeConfig.adkStreamOrigin.trim() !== "") {
    return `${runtimeConfig.adkStreamOrigin.replace(/\/$/, "")}/run_sse`;
  }
  if (import.meta.env.DEV) {
    return "/run_sse";
  }
  if (typeof window !== "undefined" && window.location?.origin) {
    return `${window.location.origin}/run_sse`;
  }
  return "http://localhost:8080/run_sse";
}

/**
 * 与 ADK 对齐：`ws(s)://{host}/run_live?app_name&user_id&session_id`
 */
export function buildAdkLiveUrl(params: {
  appName: string;
  userId: string;
  sessionId: string;
}): string {
  const protocol = typeof window !== "undefined" && window.location?.protocol === "https:" ? "wss" : "ws";
  const host = resolveAdkWsHost();
  const q = new URLSearchParams({
    app_name: params.appName,
    user_id: params.userId,
    session_id: params.sessionId
  });
  return `${protocol}://${host}/run_live?${q.toString()}`;
}

function resolveAdkWsHost(): string {
  if (runtimeConfig.adkWsHost && runtimeConfig.adkWsHost.trim() !== "") {
    return runtimeConfig.adkWsHost.trim();
  }
  if (import.meta.env.DEV) {
    return "127.0.0.1:8000";
  }
  if (typeof window !== "undefined" && window.location?.host) {
    return window.location.host;
  }
  return "127.0.0.1:8080";
}
