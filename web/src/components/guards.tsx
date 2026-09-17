import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router";
import { useI18n } from "../i18n";
import { useAuth } from "../lib/auth";
import { Button, Notice, Spinner } from "./ui";

/** Signed in, with a company: the working part of the app. */
export function RequireCompany({ children }: { children: ReactNode }) {
  const { status, user, logout } = useAuth();
  const { t } = useI18n();
  const location = useLocation();

  if (status === "loading") return <Spinner />;
  if (status === "anonymous") {
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  }
  if (user && user.roles.includes("driver") && !user.company_id) {
    return (
      <div className="mx-auto mt-20 max-w-md space-y-4 px-4">
        <Notice tone="warn">{t("auth.driverOnly")}</Notice>
        <Button variant="secondary" onClick={() => void logout()}>
          {t("nav.logout")}
        </Button>
      </div>
    );
  }
  if (!user?.company_id) return <Navigate to="/onboarding" replace />;
  return <>{children}</>;
}

/** Login and registration are for people who are not signed in yet. */
export function RequireAnonymous({ children }: { children: ReactNode }) {
  const { status } = useAuth();
  const location = useLocation();
  if (status === "loading") return <Spinner />;
  if (status === "authenticated") {
    const from = (location.state as { from?: string } | null)?.from;
    return <Navigate to={from ?? "/"} replace />;
  }
  return <>{children}</>;
}

/** Signed in but without a company yet. */
export function RequireSignedIn({ children }: { children: ReactNode }) {
  const { status, user } = useAuth();
  if (status === "loading") return <Spinner />;
  if (status === "anonymous") return <Navigate to="/login" replace />;
  if (user?.company_id) return <Navigate to="/" replace />;
  return <>{children}</>;
}
