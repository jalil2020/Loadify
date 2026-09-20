import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { lazy } from "react";
import { createBrowserRouter, Navigate, Outlet, RouterProvider } from "react-router";
import { AppShell } from "./components/AppShell";
import { ToastProvider } from "./components/feedback";
import { RequireAnonymous, RequireCompany, RequireSignedIn } from "./components/guards";
import { EmptyState, LinkButton } from "./components/ui";
import { I18nProvider, useI18n } from "./i18n";
import { ApiError } from "./lib/api";
import { AuthProvider } from "./lib/auth";
import { LoginPage } from "./pages/auth/LoginPage";
import { OnboardingPage } from "./pages/auth/OnboardingPage";
import { RegisterPage } from "./pages/auth/RegisterPage";
import { DashboardPage } from "./pages/DashboardPage";

// Signing in and the dashboard load with the app; everything else loads when
// it is first opened.
const AdsPage = lazy(() => import("./pages/ads/AdsPage").then((m) => ({ default: m.AdsPage })));
const AdEditorPage = lazy(() => import("./pages/ads/AdEditorPage").then((m) => ({ default: m.AdEditorPage })));
const AdDetailPage = lazy(() => import("./pages/ads/AdDetailPage").then((m) => ({ default: m.AdDetailPage })));
const TelegramPage = lazy(() => import("./pages/telegram/TelegramPage").then((m) => ({ default: m.TelegramPage })));
const GroupRulesPage = lazy(() =>
  import("./pages/telegram/GroupRulesPage").then((m) => ({ default: m.GroupRulesPage })),
);
const HistoryPage = lazy(() => import("./pages/HistoryPage").then((m) => ({ default: m.HistoryPage })));
const PlanPage = lazy(() => import("./pages/PlanPage").then((m) => ({ default: m.PlanPage })));
const NotificationsPage = lazy(() =>
  import("./pages/NotificationsPage").then((m) => ({ default: m.NotificationsPage })),
);

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      refetchOnWindowFocus: true,
      // A 4xx will not change on retry; only transient failures are retried.
      retry: (count, error) => !(error instanceof ApiError && error.status >= 400 && error.status < 500) && count < 2,
    },
  },
});

function NotFound() {
  const { t } = useI18n();
  return (
    <EmptyState
      title={t("err.notFoundPage")}
      action={
        <LinkButton to="/" variant="secondary">
          {t("nav.dashboard")}
        </LinkButton>
      }
    />
  );
}

function Providers() {
  return (
    <AuthProvider>
      <ToastProvider>
        <Outlet />
      </ToastProvider>
    </AuthProvider>
  );
}

const router = createBrowserRouter([
  {
    element: <Providers />,
    children: [
      { path: "/login", element: <RequireAnonymous><LoginPage /></RequireAnonymous> },
      { path: "/register", element: <RequireAnonymous><RegisterPage /></RequireAnonymous> },
      { path: "/onboarding", element: <RequireSignedIn><OnboardingPage /></RequireSignedIn> },
      {
        element: (
          <RequireCompany>
            <AppShell />
          </RequireCompany>
        ),
        children: [
          { index: true, element: <DashboardPage /> },
          { path: "ads", element: <AdsPage /> },
          { path: "ads/new", element: <AdEditorPage /> },
          { path: "ads/:id", element: <AdDetailPage /> },
          { path: "ads/:id/edit", element: <AdEditorPage /> },
          { path: "telegram", element: <TelegramPage /> },
          { path: "telegram/groups/:id", element: <GroupRulesPage /> },
          { path: "history", element: <HistoryPage /> },
          { path: "usage", element: <PlanPage /> },
          { path: "notifications", element: <NotificationsPage /> },
          { path: "dashboard", element: <Navigate to="/" replace /> },
          { path: "*", element: <NotFound /> },
        ],
      },
    ],
  },
]);

export function App() {
  return (
    <I18nProvider>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nProvider>
  );
}
