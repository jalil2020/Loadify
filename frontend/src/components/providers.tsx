"use client";

import * as React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError, api } from "@/lib/api";
import {
  clearSession,
  getSession,
  reloadSession,
  setSession,
  subscribeToSession,
  type Session,
} from "@/lib/session";
import { translate, type TranslationKey } from "@/lib/i18n";
import type { AuthResponse, CurrentUser, Locale } from "@/lib/types";

/* ── Query client ─────────────────────────────────────────────────── */

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // The load board changes constantly, but not within a few seconds of a
        // click; this avoids refetching on every incidental remount.
        staleTime: 15_000,
        retry: (failureCount, error) => {
          // Retrying a 4xx just repeats a rejection the server already decided.
          if (error instanceof ApiError && error.status < 500) return false;
          return failureCount < 2;
        },
        refetchOnWindowFocus: false,
      },
      mutations: { retry: false },
    },
  });
}

let browserQueryClient: QueryClient | undefined;

function getQueryClient() {
  // On the server every request needs its own client; in the browser one is
  // reused so a re-render does not discard the cache.
  if (typeof window === "undefined") return makeQueryClient();
  browserQueryClient ??= makeQueryClient();
  return browserQueryClient;
}

/* ── Auth ─────────────────────────────────────────────────────────── */

interface AuthContextValue {
  session: Session | null;
  user: CurrentUser | null;
  ready: boolean;
  signIn: (phone: string, password: string) => Promise<void>;
  signOut: () => void;
  can: (permission: string) => boolean;
  hasRole: (...roles: string[]) => boolean;
}

const AuthContext = React.createContext<AuthContextValue | null>(null);

function AuthProvider({ children }: { children: React.ReactNode }) {
  const [session, setLocalSession] = React.useState<Session | null>(null);
  const [ready, setReady] = React.useState(false);

  React.useEffect(() => {
    // localStorage is only readable after hydration, so the first paint is
    // deliberately "not ready" rather than wrongly signed-out.
    setLocalSession(getSession());
    setReady(true);

    const unsubscribe = subscribeToSession(setLocalSession);

    // Signing out in one tab must sign out the others.
    const onStorage = (event: StorageEvent) => {
      if (event.key === "loadify.session") setLocalSession(reloadSession());
    };
    window.addEventListener("storage", onStorage);
    return () => {
      unsubscribe();
      window.removeEventListener("storage", onStorage);
    };
  }, []);

  const signIn = React.useCallback(async (phone: string, password: string) => {
    const res = await api.post<AuthResponse>("/auth/login", { phone, password }, { anonymous: true });
    setSession({
      accessToken: res.data.tokens.access_token,
      refreshToken: res.data.tokens.refresh_token,
      expiresAt: res.data.tokens.expires_at,
      user: res.data.user,
      locale: res.data.user.locale,
    });
  }, []);

  const signOut = React.useCallback(() => {
    // Tell the backend so the session row is revoked and the access token is
    // denylisted, but drop the local session regardless of the outcome.
    api.post("/auth/logout").catch(() => undefined);
    clearSession();
    getQueryClient().clear();
  }, []);

  const value = React.useMemo<AuthContextValue>(
    () => ({
      session,
      user: session?.user ?? null,
      ready,
      signIn,
      signOut,
      can: (permission) => session?.user.permissions.includes(permission) ?? false,
      hasRole: (...roles) => roles.some((role) => session?.user.roles.includes(role)),
    }),
    [session, ready, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = React.useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside <Providers>");
  return ctx;
}

/* ── Locale ───────────────────────────────────────────────────────── */

interface LocaleContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TranslationKey) => string;
}

const LocaleContext = React.createContext<LocaleContextValue | null>(null);

function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = React.useState<Locale>("uz");

  React.useEffect(() => {
    const stored = window.localStorage.getItem("loadify.locale") as Locale | null;
    if (stored && ["uz", "ru", "en"].includes(stored)) setLocaleState(stored);
  }, []);

  const setLocale = React.useCallback((next: Locale) => {
    setLocaleState(next);
    window.localStorage.setItem("loadify.locale", next);
    document.documentElement.lang = next;

    // The API localises its own strings from Accept-Language, so cached
    // responses in the previous language have to go.
    getQueryClient().invalidateQueries();
  }, []);

  const value = React.useMemo<LocaleContextValue>(
    () => ({ locale, setLocale, t: (key) => translate(locale, key) }),
    [locale, setLocale],
  );

  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

export function useLocale(): LocaleContextValue {
  const ctx = React.useContext(LocaleContext);
  if (!ctx) throw new Error("useLocale must be used inside <Providers>");
  return ctx;
}

/* ── Root ─────────────────────────────────────────────────────────── */

export function Providers({ children }: { children: React.ReactNode }) {
  const queryClient = getQueryClient();
  return (
    <QueryClientProvider client={queryClient}>
      <LocaleProvider>
        <AuthProvider>{children}</AuthProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}
