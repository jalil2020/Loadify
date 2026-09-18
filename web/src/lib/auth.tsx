import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api, onSessionEnd, refreshSession, tokens } from "./api";
import type { AuthResponse, Company, User } from "./types";

type Status = "loading" | "anonymous" | "authenticated";

export interface RegisterInput {
  company: string;
  phone: string;
  password: string;
  locale: string;
}

export interface CompanyInput {
  legal_name: string;
  brand_name: string;
  phone: string;
}

interface Auth {
  status: Status;
  user: User | null;
  login: (phone: string, password: string) => Promise<void>;
  /** Says whether an SMS code is still owed before the person is signed in. */
  register: (input: RegisterInput) => Promise<{ verificationRequired: boolean }>;
  verify: (phone: string, code: string) => Promise<void>;
  resendCode: (phone: string) => Promise<void>;
  createCompany: (input: CompanyInput) => Promise<void>;
  logout: () => Promise<void>;
  can: (permission: string) => boolean;
  isManager: boolean;
}

const AuthContext = createContext<Auth | null>(null);
const ME = ["me"] as const;

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  // Whether there is a session to ask about; who it belongs to comes from /me.
  const [hasSession, setHasSession] = useState(() => Boolean(tokens.access() || tokens.refresh()));

  const me = useQuery({
    queryKey: ME,
    queryFn: () => api.get<User>("/auth/me"),
    enabled: hasSession,
    retry: false,
    staleTime: 5 * 60_000,
  });

  const endSession = useCallback(() => {
    tokens.clear();
    queryClient.clear();
    setHasSession(false);
  }, [queryClient]);

  useEffect(() => onSessionEnd(endSession), [endSession]);

  // Another tab signing out ends this one too.
  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === "loadify.refresh" && e.newValue === null) endSession();
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, [endSession]);

  const signedIn = useCallback(
    (res: AuthResponse) => {
      tokens.set(res.tokens);
      queryClient.setQueryData(ME, res.user);
      setHasSession(true);
    },
    [queryClient],
  );

  const login = useCallback(
    async (phone: string, password: string) => {
      const res = await api.post<AuthResponse>("/auth/login", { phone, password, platform: "web" }, { anonymous: true });
      signedIn(res);
    },
    [signedIn],
  );

  // One step: the account, then its company, then a session that names the
  // company. The session is only switched on at the end, so the person goes
  // straight to the dashboard instead of passing through the company form.
  const register = useCallback(
    async (input: RegisterInput) => {
      const res = await api.post<{ verification_required: boolean; auth?: AuthResponse }>(
        "/auth/register",
        { phone: input.phone, password: input.password, locale: input.locale, role: "company" },
        { anonymous: true },
      );
      if (!res.auth) return { verificationRequired: true };

      tokens.set(res.auth.tokens);
      try {
        await api.post<Company>("/companies", {
          legal_name: input.company,
          brand_name: input.company,
          phone: input.phone,
        });
        await refreshSession();
        queryClient.setQueryData(ME, await api.get<User>("/auth/me"));
      } catch {
        // The account exists either way; without a company the person lands
        // on the company form and can try again from there.
        queryClient.setQueryData(ME, res.auth.user);
      }
      setHasSession(true);
      return { verificationRequired: false };
    },
    [queryClient],
  );

  const verify = useCallback(
    async (phone: string, code: string) => {
      const res = await api.post<{ verified: boolean; auth?: AuthResponse }>(
        "/auth/verify-otp",
        { phone, code, purpose: "register" },
        { anonymous: true },
      );
      if (res.auth) signedIn(res.auth);
    },
    [signedIn],
  );

  const resendCode = useCallback(async (phone: string) => {
    await api.post("/auth/resend-otp", { phone, purpose: "register" }, { anonymous: true });
  }, []);

  const createCompany = useCallback(
    async (input: CompanyInput) => {
      await api.post<Company>("/companies", input);
      // The access token names the company, so a new one is needed before
      // anything company-scoped will answer.
      await refreshSession();
      await queryClient.invalidateQueries();
      await queryClient.refetchQueries({ queryKey: ME });
    },
    [queryClient],
  );

  const logout = useCallback(async () => {
    try {
      await api.post("/auth/logout", { refresh_token: tokens.refresh() ?? undefined });
    } catch {
      // Signing out locally is what matters; the server session expires anyway.
    }
    endSession();
  }, [endSession]);

  const user = hasSession ? (me.data ?? null) : null;
  const status: Status = !hasSession || me.isError ? "anonymous" : user ? "authenticated" : "loading";

  const value = useMemo<Auth>(() => {
    const permissions = new Set(user?.permissions ?? []);
    return {
      status,
      user,
      login,
      register,
      verify,
      resendCode,
      createCompany,
      logout,
      can: (permission) => permissions.has(permission),
      isManager: user?.company_role === "company_manager" || (user?.roles ?? []).includes("company_manager"),
    };
  }, [status, user, login, register, verify, resendCode, createCompany, logout]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): Auth {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth outside AuthProvider");
  return ctx;
}
