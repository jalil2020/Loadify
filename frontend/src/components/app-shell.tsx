"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuth, useLocale } from "@/components/providers";
import { Button } from "@/components/ui/primitives";
import { LOCALES } from "@/lib/i18n";
import type { TranslationKey } from "@/lib/i18n";
import { initials } from "@/lib/format";
import { cn } from "@/lib/utils";

interface NavItem {
  href: string;
  labelKey: TranslationKey;
  /** Shown only when the signed-in principal holds this permission. */
  permission?: string;
  /** Shown only for these roles. */
  roles?: string[];
  /** Shown only when signed in. */
  authed?: boolean;
}

const NAV: NavItem[] = [
  { href: "/", labelKey: "nav.board" },
  { href: "/my-loads", labelKey: "nav.myLoads", permission: "loads.create" },
  { href: "/bookings", labelKey: "nav.bookings", permission: "bookings.read" },
  { href: "/trips", labelKey: "nav.trips", permission: "trips.read" },
  { href: "/drivers", labelKey: "nav.drivers", permission: "drivers.search" },
  { href: "/wallet", labelKey: "nav.wallet", permission: "payments.read" },
  { href: "/admin", labelKey: "nav.admin", roles: ["admin", "super_admin"] },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const { user, ready, signOut, can, hasRole } = useAuth();
  const { locale, setLocale, t } = useLocale();
  const pathname = usePathname();
  const router = useRouter();
  const [menuOpen, setMenuOpen] = React.useState(false);

  const visibleNav = NAV.filter((item) => {
    if (item.permission) return can(item.permission);
    if (item.roles) return hasRole(...item.roles);
    if (item.authed) return Boolean(user);
    return true;
  });

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-40 border-b border-border bg-card/85 backdrop-blur">
        <div className="container flex h-14 items-center gap-4">
          <Link href="/" className="flex items-center gap-2 font-semibold">
            <Logo className="h-7 w-7" />
            <span className="text-[15px]">{t("app.name")}</span>
          </Link>

          <nav className="hidden items-center gap-1 md:flex">
            {visibleNav.map((item) => {
              const active =
                item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                    active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                  )}
                >
                  {t(item.labelKey)}
                </Link>
              );
            })}
          </nav>

          <div className="ml-auto flex items-center gap-2">
            <select
              aria-label="Language"
              value={locale}
              onChange={(e) => setLocale(e.target.value as typeof locale)}
              className="h-8 rounded-md border border-input bg-card px-2 text-xs"
            >
              {LOCALES.map((l) => (
                <option key={l.code} value={l.code}>
                  {l.label}
                </option>
              ))}
            </select>

            {/* Until the session is read from storage, render neither state —
                flashing "Sign in" at an authenticated user is worse than a beat
                of nothing. */}
            {!ready ? (
              <div className="h-8 w-20 skeleton" />
            ) : user ? (
              <div className="flex items-center gap-2">
                <div
                  title={user.full_name}
                  className="hidden h-8 w-8 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground sm:flex"
                >
                  {initials(user.full_name)}
                </div>
                <Button variant="ghost" size="sm" onClick={() => { signOut(); router.push("/"); }}>
                  {t("nav.signOut")}
                </Button>
              </div>
            ) : (
              <Button size="sm" onClick={() => router.push("/login")}>
                {t("nav.signIn")}
              </Button>
            )}

            <button
              className="rounded-md p-2 hover:bg-muted md:hidden"
              aria-label="Menu"
              aria-expanded={menuOpen}
              onClick={() => setMenuOpen((v) => !v)}
            >
              <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M4 6h16M4 12h16M4 18h16" strokeLinecap="round" />
              </svg>
            </button>
          </div>
        </div>

        {menuOpen && (
          <nav className="container flex flex-col gap-1 border-t border-border py-2 md:hidden">
            {visibleNav.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                onClick={() => setMenuOpen(false)}
                className="rounded-md px-3 py-2 text-sm font-medium hover:bg-muted"
              >
                {t(item.labelKey)}
              </Link>
            ))}
          </nav>
        )}
      </header>

      <main className="container flex-1 py-6">{children}</main>

      <footer className="border-t border-border py-5">
        <div className="container flex flex-col items-center justify-between gap-2 text-xs text-muted-foreground sm:flex-row">
          <span>© {new Date().getFullYear()} Loadify</span>
          <span>{t("app.tagline")}</span>
        </div>
      </footer>
    </div>
  );
}

/** The logo mark: a package moving right, in the brand navy and green. */
function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden>
      <path d="M5 6h6v14h9v6H5z" fill="hsl(var(--primary))" />
      <path d="M20 12h7l4 5-4 5h-7z" fill="hsl(var(--accent))" />
      <rect x="13" y="6" width="9" height="9" rx="1.5" fill="hsl(var(--primary))" opacity=".85" />
    </svg>
  );
}
