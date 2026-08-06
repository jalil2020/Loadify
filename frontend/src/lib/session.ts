"use client";

import type { CurrentUser } from "@/lib/types";

export interface Session {
  accessToken: string;
  refreshToken: string;
  expiresAt: string;
  user: CurrentUser;
  locale?: string;
}

const STORAGE_KEY = "loadify.session";

/**
 * The session lives in localStorage.
 *
 * A production deployment should move the refresh token to an httpOnly cookie
 * issued by a Next route handler, so XSS cannot read it. That needs the BFF
 * route to exist; until then this is the honest trade-off, and it is why the
 * refresh token is never logged or put in a URL.
 */
let cached: Session | null | undefined;

const listeners = new Set<(session: Session | null) => void>();

export function getSession(): Session | null {
  if (typeof window === "undefined") return null;
  if (cached !== undefined) return cached;

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    cached = raw ? (JSON.parse(raw) as Session) : null;
  } catch {
    cached = null;
  }
  return cached;
}

export function setSession(session: Session): void {
  cached = session;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
  } catch {
    // A full or disabled storage must not break the signed-in session that is
    // already held in memory.
  }
  listeners.forEach((listener) => listener(session));
}

export function clearSession(): void {
  cached = null;
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
  listeners.forEach((listener) => listener(null));
}

export function subscribeToSession(listener: (session: Session | null) => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Reads the session without the module cache, for cross-tab sync. */
export function reloadSession(): Session | null {
  cached = undefined;
  return getSession();
}
