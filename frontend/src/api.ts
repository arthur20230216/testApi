const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "http://localhost:8080";

function buildApiUrl(path: string): string {
  const base = API_BASE_URL.replace(/\/+$/, "");
  let normalizedPath = path.startsWith("/") ? path : `/${path}`;

  // Avoid duplicated "/api" when base is "/api" and caller passes "/api/*".
  if (base.endsWith("/api") && normalizedPath.startsWith("/api/")) {
    normalizedPath = normalizedPath.slice(4);
  }

  return `${base}${normalizedPath}`;
}

async function readPayload(response: Response): Promise<unknown> {
  const contentType = response.headers.get("content-type") ?? "";
  const text = await response.text();

  if (contentType.includes("application/json")) {
    try {
      return JSON.parse(text);
    } catch {
      throw new Error(`服务端返回了无效 JSON（HTTP ${response.status}）`);
    }
  }

  const compact = text.replace(/\s+/g, " ").trim();
  const excerpt = compact.length > 120 ? `${compact.slice(0, 120)}...` : compact;
  throw new Error(`接口返回非 JSON（HTTP ${response.status}）：${excerpt || "empty body"}`);
}

function extractError(payload: unknown, fallback: string): string {
  if (typeof payload !== "object" || payload === null) {
    return fallback;
  }

  const maybeError = (payload as { detail?: unknown; error?: unknown });
  if (typeof maybeError.detail === "string" && maybeError.detail.length > 0) {
    return maybeError.detail;
  }
  if (typeof maybeError.error === "string" && maybeError.error.length > 0) {
    return maybeError.error;
  }

  return fallback;
}

export async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(buildApiUrl(path));
  const payload = await readPayload(response);

  if (!response.ok) {
    throw new Error(extractError(payload, `Request failed (HTTP ${response.status})`));
  }

  return payload as T;
}

export async function apiPost<T>(path: string, body: unknown): Promise<T> {
  return requestJson<T>("POST", path, body);
}

export async function apiPatch<T>(path: string, body: unknown): Promise<T> {
  return requestJson<T>("PATCH", path, body);
}

export async function apiDelete<T>(path: string): Promise<T> {
  return requestJson<T>("DELETE", path);
}

async function requestJson<T>(method: string, path: string, body?: unknown): Promise<T> {
  const response = await fetch(buildApiUrl(path), {
    method,
    headers: {
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const payload = await readPayload(response);

  if (!response.ok) {
    throw new Error(extractError(payload, `Request failed (HTTP ${response.status})`));
  }

  return payload as T;
}
