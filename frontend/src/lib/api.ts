import { getSession, setSession, clearSession } from "@/lib/session";
import type { AuthResponse } from "@/lib/types";

export const API_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api/v1";

/** The envelope every endpoint returns. */
export interface Envelope<T> {
  success: boolean;
  data: T;
  meta?: PageMeta;
}

export interface PageMeta {
  page: number;
  per_page: number;
  total: number;
  total_pages: number;
}

export interface FieldError {
  field: string;
  message: string;
}

/**
 * A failed request, carrying the backend's stable machine code.
 *
 * Callers branch on `code`, never on `message` — messages are localised and
 * change; codes are part of the contract.
 */
export class ApiError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
    readonly details: FieldError[] = [],
    readonly traceId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }

  /** Maps validation details onto react-hook-form's setError shape. */
  get fieldErrors(): Record<string, string> {
    return Object.fromEntries(this.details.map((d) => [d.field, d.message]));
  }

  get isValidation() {
    return this.code === "VALIDATION_ERROR";
  }
  get isAuth() {
    return this.status === 401;
  }
}

/**
 * Query parameters.
 *
 * A plain interface is accepted as well as an index-signature type, so callers
 * can pass their own typed filter shape without casting.
 */
export type QueryParams = Record<string, unknown>;

interface RequestOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
  /** Query parameters; undefined, null and empty values are dropped. */
  params?: object;
  /** Skip the Authorization header, for genuinely public endpoints. */
  anonymous?: boolean;
  /** Internal: prevents a refresh loop. */
  _retried?: boolean;
}

/**
 * Concurrent 401s share one refresh.
 *
 * Without this, a dashboard firing six queries at once would send six refresh
 * requests; the backend rotates the token on first use, so the other five would
 * present an already-rotated token and trip its theft detection — logging the
 * user out for doing nothing wrong.
 */
let refreshInFlight: Promise<boolean> | null = null;

async function refreshTokens(): Promise<boolean> {
  const session = getSession();
  if (!session?.refreshToken) return false;

  try {
    const res = await fetch(`${API_URL}/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: session.refreshToken }),
    });
    if (!res.ok) {
      clearSession();
      return false;
    }
    const payload = (await res.json()) as Envelope<AuthResponse>;
    const tokens = payload.data?.tokens;

    // A 200 with an unreadable body is treated as a failed refresh. Storing
    // undefined here would be worse than failing: every later request would go
    // out with no Authorization header and be rejected as anonymous, which
    // reads as a broken app rather than an expired session.
    if (!tokens?.access_token || !tokens.refresh_token) {
      clearSession();
      return false;
    }

    setSession({
      accessToken: tokens.access_token,
      refreshToken: tokens.refresh_token,
      expiresAt: tokens.expires_at,
      // Roles and permissions can change between refreshes, so the refreshed
      // user is preferred over the copy held since sign-in.
      user: payload.data.user ?? session.user,
      locale: session.locale,
    });
    return true;
  } catch {
    clearSession();
    return false;
  }
}

function ensureRefresh(): Promise<boolean> {
  refreshInFlight ??= refreshTokens().finally(() => {
    refreshInFlight = null;
  });
  return refreshInFlight;
}

function buildQuery(params?: object): string {
  if (!params) return "";
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params) as [string, unknown][]) {
    if (value === undefined || value === null || value === "") continue;
    // Repeated keys are how the API takes list filters: ?vehicle_type=a&vehicle_type=b
    if (Array.isArray(value)) {
      value.filter((v) => v !== undefined && v !== null && v !== "").forEach((v) => search.append(key, String(v)));
    } else {
      search.set(key, String(value));
    }
  }
  const qs = search.toString();
  return qs ? `?${qs}` : "";
}

export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {},
): Promise<Envelope<T>> {
  const { body, params, anonymous, _retried, headers, ...rest } = options;
  const session = getSession();

  const requestHeaders = new Headers(headers);
  if (!requestHeaders.has("Accept-Language")) {
    requestHeaders.set("Accept-Language", getSession()?.locale ?? "uz");
  }
  if (body !== undefined && !(body instanceof FormData)) {
    requestHeaders.set("Content-Type", "application/json");
  }
  if (!anonymous && session?.accessToken) {
    requestHeaders.set("Authorization", `Bearer ${session.accessToken}`);
  }

  const res = await fetch(`${API_URL}${path}${buildQuery(params)}`, {
    ...rest,
    headers: requestHeaders,
    body:
      body === undefined
        ? undefined
        : body instanceof FormData
          ? body
          : JSON.stringify(body),
  });

  if (res.status === 204) {
    return { success: true, data: undefined as T };
  }

  let payload: unknown;
  try {
    payload = await res.json();
  } catch {
    throw new ApiError("NETWORK_ERROR", "The server returned an unreadable response", res.status);
  }

  if (!res.ok) {
    const error = (payload as { error?: { code: string; message: string; details?: FieldError[] }; trace_id?: string }).error;
    const code = error?.code ?? "UNKNOWN";

    // One retry after a successful refresh. TOKEN_REUSE_DETECTED is excluded:
    // the backend has already revoked the whole family, so retrying is futile
    // and the only correct response is to sign in again.
    if (res.status === 401 && !_retried && !anonymous && code !== "TOKEN_REUSE_DETECTED") {
      if (await ensureRefresh()) {
        return apiFetch<T>(path, { ...options, _retried: true });
      }
    }

    throw new ApiError(
      code,
      error?.message ?? "Something went wrong",
      res.status,
      error?.details ?? [],
      (payload as { trace_id?: string }).trace_id,
    );
  }

  return payload as Envelope<T>;
}

export const api = {
  get: <T>(path: string, params?: object, options?: RequestOptions) =>
    apiFetch<T>(path, { ...options, method: "GET", params }),
  post: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    apiFetch<T>(path, { ...options, method: "POST", body }),
  put: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    apiFetch<T>(path, { ...options, method: "PUT", body }),
  patch: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    apiFetch<T>(path, { ...options, method: "PATCH", body }),
  delete: <T>(path: string, options?: RequestOptions) =>
    apiFetch<T>(path, { ...options, method: "DELETE" }),
};
