import type { AuthResponse, PageMeta, TokenPair } from "./types";

const BASE = "/api/v1";
const ACCESS_KEY = "loadify.access";
const REFRESH_KEY = "loadify.refresh";

/** An error the API answered with, or a network failure (status 0). */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: { field: string; message: string }[];

  constructor(status: number, code: string, message: string, details: { field: string; message: string }[] = []) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

// ── tokens ──────────────────────────────────────────────────────────
// Both tokens live in localStorage so every tab shares one session. The
// refresh token rotates on every use and the server revokes the whole session
// when an old one comes back, so a refresh is always done under a cross-tab
// lock and re-reads storage first.

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // Storage can be unavailable (private mode); the session then lasts as
    // long as this page.
  }
}

let memoryAccess: string | null = null;
let memoryRefresh: string | null = null;

export const tokens = {
  access: () => read(ACCESS_KEY) ?? memoryAccess,
  refresh: () => read(REFRESH_KEY) ?? memoryRefresh,
  set(pair: TokenPair) {
    memoryAccess = pair.access_token;
    memoryRefresh = pair.refresh_token;
    write(ACCESS_KEY, pair.access_token);
    write(REFRESH_KEY, pair.refresh_token);
  },
  clear() {
    memoryAccess = null;
    memoryRefresh = null;
    write(ACCESS_KEY, null);
    write(REFRESH_KEY, null);
  },
};

type SessionListener = () => void;
const sessionListeners = new Set<SessionListener>();

/** Called when the session can no longer be renewed. */
export function onSessionEnd(listener: SessionListener): () => void {
  sessionListeners.add(listener);
  return () => sessionListeners.delete(listener);
}

let locale = "uz";
export function setApiLocale(value: string) {
  locale = value;
}

let refreshing: Promise<boolean> | null = null;

async function withLock<T>(fn: () => Promise<T>): Promise<T> {
  if (typeof navigator !== "undefined" && navigator.locks) {
    return navigator.locks.request("loadify-token-refresh", fn);
  }
  return fn();
}

/** Renews the access token once, however many requests ask at the same time. */
export function refreshSession(): Promise<boolean> {
  refreshing ??= (async () => {
    const before = tokens.refresh();
    try {
      return await withLock(async () => {
        const current = tokens.refresh();
        if (!current) return false;
        // Another tab rotated the token while this one waited: use its result.
        if (before && current !== before) return true;

        const res = await fetch(`${BASE}/auth/refresh`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: current }),
        });
        if (!res.ok) {
          tokens.clear();
          return false;
        }
        const body = (await res.json()) as { data: AuthResponse };
        tokens.set(body.data.tokens);
        return true;
      });
    } catch {
      return false;
    } finally {
      refreshing = null;
    }
  })();
  return refreshing;
}

// ── requests ────────────────────────────────────────────────────────
type Query = Record<string, string | number | boolean | undefined | null>;

interface RequestOptions {
  body?: unknown;
  query?: Query;
  headers?: Record<string, string>;
  /** Public endpoints skip the token and the refresh dance. */
  anonymous?: boolean;
}

interface Envelope<T> {
  success: boolean;
  data: T;
  meta?: PageMeta;
  error?: { code: string; message: string; details?: { field: string; message: string }[] };
}

function url(path: string, query?: Query): string {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v !== undefined && v !== null && v !== "") params.set(k, String(v));
  }
  const qs = params.toString();
  return `${BASE}${path}${qs ? `?${qs}` : ""}`;
}

async function send(method: string, path: string, opts: RequestOptions): Promise<Response> {
  const headers: Record<string, string> = { "Accept-Language": locale, ...opts.headers };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  const access = opts.anonymous ? null : tokens.access();
  if (access) headers.Authorization = `Bearer ${access}`;

  try {
    return await fetch(url(path, opts.query), {
      method,
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });
  } catch {
    throw new ApiError(0, "NETWORK", "network");
  }
}

async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<Envelope<T>> {
  let res = await send(method, path, opts);

  if (res.status === 401 && !opts.anonymous) {
    if (await refreshSession()) {
      res = await send(method, path, opts);
    } else {
      sessionListeners.forEach((l) => l());
    }
  }

  if (res.status === 204) {
    return { success: true, data: undefined as T };
  }

  const body = (await res.json().catch(() => null)) as Envelope<T> | null;

  if (!res.ok || !body || body.success === false) {
    const err = body?.error;
    if (res.status === 401 && !opts.anonymous) sessionListeners.forEach((l) => l());
    throw new ApiError(res.status, err?.code ?? "UNKNOWN", err?.message ?? res.statusText, err?.details ?? []);
  }
  return body;
}

export const api = {
  async get<T>(path: string, query?: Query, opts?: RequestOptions): Promise<T> {
    return (await request<T>("GET", path, { ...opts, query })).data;
  },
  async page<T>(path: string, query?: Query): Promise<{ items: T[]; meta: PageMeta }> {
    const res = await request<T[]>("GET", path, { query });
    return {
      items: res.data ?? [],
      meta: res.meta ?? { page: 1, per_page: res.data?.length ?? 0, total: res.data?.length ?? 0, total_pages: 1 },
    };
  },
  async post<T>(path: string, body?: unknown, opts?: RequestOptions): Promise<T> {
    return (await request<T>("POST", path, { ...opts, body: body ?? {} })).data;
  },
  async put<T>(path: string, body: unknown): Promise<T> {
    return (await request<T>("PUT", path, { body })).data;
  },
  async del(path: string): Promise<void> {
    await request<void>("DELETE", path);
  },
};
