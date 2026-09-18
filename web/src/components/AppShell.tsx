import { clsx } from "clsx";
import { Bell, FileText, Gauge, History, LayoutDashboard, LogOut, Menu, Send, X } from "lucide-react";
import { Suspense, useEffect, useState, type ReactNode } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router";
import { LANGS, useI18n, type Lang, type MessageKey } from "../i18n";
import { useAuth } from "../lib/auth";
import { useUnreadCount } from "../lib/queries";
import { Spinner } from "./ui";

const NAV: { to: string; label: MessageKey; icon: ReactNode; end?: boolean }[] = [
  { to: "/", label: "nav.dashboard", icon: <LayoutDashboard className="size-4" />, end: true },
  { to: "/ads", label: "nav.ads", icon: <FileText className="size-4" /> },
  { to: "/telegram", label: "nav.telegram", icon: <Send className="size-4" /> },
  { to: "/history", label: "nav.history", icon: <History className="size-4" /> },
  { to: "/usage", label: "nav.usage", icon: <Gauge className="size-4" /> },
];

function Brand() {
  return (
    <Link to="/" className="flex items-center gap-2 font-semibold text-ink">
      <span className="grid size-8 place-items-center rounded-lg bg-brand text-on-brand">
        <Send className="size-4" aria-hidden />
      </span>
      Loadify
    </Link>
  );
}

function NavItems({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useI18n();
  return (
    <ul className="flex flex-col gap-1">
      {NAV.map((item) => (
        <li key={item.to}>
          <NavLink
            to={item.to}
            end={item.end}
            onClick={onNavigate}
            className={({ isActive }) =>
              clsx(
                "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                isActive ? "bg-brand-soft text-brand-strong" : "text-ink-soft hover:bg-sunken",
              )
            }
          >
            {item.icon}
            {t(item.label)}
          </NavLink>
        </li>
      ))}
    </ul>
  );
}

function LanguageSwitch() {
  const { lang, setLang, t } = useI18n();
  return (
    <div role="group" aria-label="Language" className="flex rounded-lg border border-line p-0.5">
      {LANGS.map((l: Lang) => (
        <button
          key={l}
          type="button"
          aria-pressed={lang === l}
          title={t(`lang.${l}`)}
          onClick={() => setLang(l)}
          className={clsx(
            "rounded-md px-2 py-1 text-xs font-semibold uppercase",
            lang === l ? "bg-sunken text-ink" : "text-muted hover:text-ink",
          )}
        >
          {l}
        </button>
      ))}
    </div>
  );
}

function NotificationBell() {
  const { t } = useI18n();
  const unread = useUnreadCount();
  const count = unread.data?.notifications ?? 0;
  return (
    <Link
      to="/notifications"
      className="relative rounded-lg p-2 text-ink-soft hover:bg-sunken"
      aria-label={`${t("nav.notifications")}${count ? ` (${count})` : ""}`}
    >
      <Bell className="size-5" aria-hidden />
      {count > 0 && (
        <span className="absolute -top-0.5 -right-0.5 min-w-5 rounded-full bg-bad px-1 text-center text-[11px] font-semibold leading-5 text-white tabular">
          {count > 99 ? "99+" : count}
        </span>
      )}
    </Link>
  );
}

export function AppShell() {
  const { t } = useI18n();
  const { user, logout } = useAuth();
  const location = useLocation();
  // The mobile menu belongs to the page it was opened on, so navigating closes it.
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const menuOpen = menuFor === location.pathname;
  const setMenuOpen = (open: boolean) => setMenuFor(open ? location.pathname : null);

  // Focus the page heading after navigation so screen readers announce it.
  useEffect(() => {
    document.querySelector<HTMLElement>("main h1")?.focus?.();
  }, [location.pathname]);

  return (
    <div className="flex min-h-full">
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-line bg-surface px-3 py-4 lg:flex">
        <div className="px-2 pb-6">
          <Brand />
        </div>
        <nav aria-label={t("nav.menu")} className="flex-1">
          <NavItems />
        </nav>
        <div className="border-t border-line px-2 pt-3">
          <p className="truncate text-sm font-medium text-ink">{user?.full_name.trim() || user?.phone}</p>
          {user?.full_name.trim() && <p className="truncate text-xs text-muted">{user.phone}</p>}
          <button
            type="button"
            onClick={() => void logout()}
            className="mt-3 flex items-center gap-2 text-sm text-ink-soft hover:text-bad"
          >
            <LogOut className="size-4" aria-hidden />
            {t("nav.logout")}
          </button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-3 border-b border-line bg-surface/90 px-4 backdrop-blur">
          <div className="flex items-center gap-2 lg:hidden">
            <button
              type="button"
              className="rounded-lg p-2 text-ink-soft hover:bg-sunken"
              aria-label={t("nav.menu")}
              aria-expanded={menuOpen}
              onClick={() => setMenuOpen(!menuOpen)}
            >
              {menuOpen ? <X className="size-5" /> : <Menu className="size-5" />}
            </button>
            <Brand />
          </div>
          <div className="hidden lg:block" />
          <div className="flex items-center gap-2">
            <LanguageSwitch />
            <NotificationBell />
          </div>
        </header>

        {menuOpen && (
          <div className="border-b border-line bg-surface px-3 py-3 lg:hidden">
            <nav aria-label={t("nav.menu")}>
              <NavItems onNavigate={() => setMenuOpen(false)} />
            </nav>
            <button
              type="button"
              onClick={() => void logout()}
              className="mt-2 flex items-center gap-2 px-3 py-2 text-sm text-ink-soft"
            >
              <LogOut className="size-4" aria-hidden />
              {t("nav.logout")}
            </button>
          </div>
        )}

        <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 sm:px-6 lg:py-8">
          <Suspense fallback={<Spinner />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    </div>
  );
}
