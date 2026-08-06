"use client";

import * as React from "react";
import { useRouter, usePathname } from "next/navigation";
import { useAuth, useLocale } from "@/components/providers";
import { Skeleton } from "@/components/ui/primitives";

/**
 * Client-side gating for a page.
 *
 * This is a UX guard, not a security boundary: every endpoint enforces its own
 * permission server-side. Its job is to avoid rendering a screen that would
 * only produce 403s.
 */
export function RequireAuth({
  permission,
  roles,
  children,
}: {
  permission?: string;
  roles?: string[];
  children: React.ReactNode;
}) {
  const { user, ready, can, hasRole } = useAuth();
  const { t } = useLocale();
  const router = useRouter();
  const pathname = usePathname();

  React.useEffect(() => {
    if (!ready) return;
    if (!user) router.replace(`/login?next=${encodeURIComponent(pathname)}`);
  }, [ready, user, router, pathname]);

  if (!ready || !user) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-32 w-full" />
      </div>
    );
  }

  const allowed =
    (!permission || can(permission)) && (!roles || hasRole(...roles));

  if (!allowed) {
    return (
      <div className="rounded-lg border border-border bg-card px-6 py-12 text-center">
        <p className="font-medium">{t("common.error")}</p>
        <p className="mt-1 text-sm text-muted-foreground">403</p>
      </div>
    );
  }

  return <>{children}</>;
}
