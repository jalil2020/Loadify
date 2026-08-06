"use client";

import { RequireAuth } from "@/components/guards";
import { useLocale } from "@/components/providers";
import { Card, CardBody, CardHeader, CardTitle, ErrorState, Skeleton } from "@/components/ui/primitives";
import { useCorridors, useDashboard } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatMoneyCompact, formatNumber } from "@/lib/format";

export default function AdminPage() {
  return (
    <RequireAuth roles={["admin", "super_admin"]}>
      <AdminView />
    </RequireAuth>
  );
}

function AdminView() {
  const { t, locale } = useLocale();
  const dashboard = useDashboard();
  const corridors = useCorridors();

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold tracking-tight">{t("admin.dashboard")}</h1>

      {dashboard.isError && (
        <ErrorState
          message={dashboard.error instanceof ApiError ? dashboard.error.message : t("common.error")}
          onRetry={() => dashboard.refetch()}
          retryLabel={t("common.retry")}
        />
      )}

      {dashboard.isPending && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 8 }, (_, i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
      )}

      {dashboard.isSuccess && (
        <>
          <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Tile label={t("admin.gmv")} value={formatMoneyCompact(dashboard.data.gmv, "UZS", locale)} accent />
            <Tile
              label={t("admin.commission")}
              value={formatMoneyCompact(dashboard.data.commission_earned, "UZS", locale)}
              accent
            />
            <Tile label="Published loads" value={formatNumber(dashboard.data.published_loads, locale)} />
            <Tile label="Active trips" value={formatNumber(dashboard.data.active_trips, locale)} />
          </section>

          <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Tile
              label="Drivers"
              value={formatNumber(dashboard.data.drivers, locale)}
              sub={`${formatNumber(dashboard.data.online_drivers, locale)} online · ${formatNumber(dashboard.data.verified_drivers, locale)} verified`}
            />
            <Tile
              label="Companies"
              value={formatNumber(dashboard.data.companies, locale)}
              sub={`${formatNumber(dashboard.data.verified_companies, locale)} verified`}
            />
            <Tile
              label="Completed trips"
              value={formatNumber(dashboard.data.completed_trips, locale)}
            />
            {/* The queues are what an operator actually acts on, so they sit
                beside the vanity counts rather than on a separate screen. */}
            <Tile
              label="Needs attention"
              value={formatNumber(
                dashboard.data.pending_documents +
                  dashboard.data.open_complaints +
                  dashboard.data.pending_refunds,
                locale,
              )}
              sub={`${dashboard.data.pending_documents} docs · ${dashboard.data.open_complaints} complaints · ${dashboard.data.pending_refunds} refunds`}
              warn={
                dashboard.data.pending_documents +
                  dashboard.data.open_complaints +
                  dashboard.data.pending_refunds >
                0
              }
            />
          </section>
        </>
      )}

      <Card>
        <CardHeader>
          <CardTitle>{t("admin.corridors")}</CardTitle>
        </CardHeader>
        <CardBody className="px-0 py-0">
          {corridors.isPending && (
            <div className="space-y-2 p-5">
              {Array.from({ length: 3 }, (_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </div>
          )}

          {corridors.isSuccess && corridors.data.length === 0 && (
            <p className="p-5 text-sm text-muted-foreground">{t("board.empty")}</p>
          )}

          {corridors.isSuccess && corridors.data.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[34rem] text-sm">
                <thead className="border-b border-border bg-muted/40 text-left">
                  <tr className="text-xs uppercase tracking-wide text-muted-foreground">
                    <th className="px-5 py-2.5 font-medium">Route</th>
                    <th className="px-5 py-2.5 font-medium">Loads</th>
                    <th className="px-5 py-2.5 font-medium">Trips</th>
                    <th className="px-5 py-2.5 font-medium">Fill rate</th>
                  </tr>
                </thead>
                <tbody>
                  {corridors.data.map((c, i) => (
                    <tr key={i} className="border-b border-border last:border-0">
                      <td className="px-5 py-2.5">
                        {c.pickup_region} → {c.delivery_region}
                      </td>
                      <td className="tnum px-5 py-2.5">{formatNumber(c.loads, locale)}</td>
                      <td className="tnum px-5 py-2.5">{formatNumber(c.trips, locale)}</td>
                      <td className="px-5 py-2.5">
                        <div className="flex items-center gap-2">
                          <div className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full bg-accent"
                              style={{ width: `${Math.min(100, c.fill_rate)}%` }}
                            />
                          </div>
                          <span className="tnum text-xs">{c.fill_rate.toFixed(0)}%</span>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardBody>
      </Card>
    </div>
  );
}

function Tile({
  label,
  value,
  sub,
  accent,
  warn,
}: {
  label: string;
  value: string;
  sub?: string;
  accent?: boolean;
  warn?: boolean;
}) {
  return (
    <Card>
      <CardBody>
        <p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
        <p
          className={`tnum mt-1 text-2xl font-semibold ${
            warn ? "text-warning" : accent ? "text-accent" : ""
          }`}
        >
          {value}
        </p>
        {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
      </CardBody>
    </Card>
  );
}
