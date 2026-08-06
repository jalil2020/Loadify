"use client";

import * as React from "react";
import { RequireAuth } from "@/components/guards";
import { useLocale } from "@/components/providers";
import { Pagination } from "@/components/pagination";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorState,
  Select,
  Skeleton,
} from "@/components/ui/primitives";
import { useMyLoads, usePublishLoad, type LoadFilters } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatDate, formatMoney, formatWeight } from "@/lib/format";
import type { LoadStatus } from "@/lib/types";

const STATUSES: LoadStatus[] = [
  "draft",
  "published",
  "reserved",
  "assigned",
  "in_transit",
  "delivered",
  "completed",
  "cancelled",
  "expired",
];

const TONE: Record<LoadStatus, "neutral" | "primary" | "success" | "warning" | "danger" | "accent"> = {
  draft: "neutral",
  published: "accent",
  reserved: "warning",
  assigned: "primary",
  in_transit: "primary",
  delivered: "success",
  completed: "success",
  cancelled: "danger",
  expired: "neutral",
};

export default function MyLoadsPage() {
  return (
    <RequireAuth permission="loads.create">
      <MyLoadsView />
    </RequireAuth>
  );
}

function MyLoadsView() {
  const { t, locale } = useLocale();
  const [filters, setFilters] = React.useState<LoadFilters>({ page: 1 });
  const [error, setError] = React.useState<string | null>(null);

  const loads = useMyLoads(filters);
  const publish = usePublishLoad();

  const onPublish = async (loadId: string) => {
    setError(null);
    try {
      await publish.mutateAsync(loadId);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"));
    }
  };

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">{t("loads.mine")}</h1>
        <Select
          className="w-44"
          value={filters.status ?? ""}
          onChange={(e) => setFilters({ page: 1, status: e.target.value || undefined })}
        >
          <option value="">{t("common.all")}</option>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {s.replace(/_/g, " ")}
            </option>
          ))}
        </Select>
      </header>

      {error && <ErrorState message={error} retryLabel={t("common.retry")} />}

      {loads.isError && (
        <ErrorState
          message={loads.error instanceof ApiError ? loads.error.message : t("common.error")}
          onRetry={() => loads.refetch()}
          retryLabel={t("common.retry")}
        />
      )}

      {loads.isPending && (
        <div className="space-y-3">
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
      )}

      {loads.isSuccess && loads.data.items.length === 0 && (
        <EmptyState title={t("board.empty")} hint={t("board.emptyHint")} />
      )}

      {loads.isSuccess && loads.data.items.length > 0 && (
        <>
          <Card className="overflow-x-auto">
            <table className="w-full min-w-[46rem] text-sm">
              <thead className="border-b border-border bg-muted/40 text-left">
                <tr className="text-xs uppercase tracking-wide text-muted-foreground">
                  <th className="px-4 py-2.5 font-medium">Ref</th>
                  <th className="px-4 py-2.5 font-medium">Route</th>
                  <th className="px-4 py-2.5 font-medium">{t("load.weight")}</th>
                  <th className="px-4 py-2.5 font-medium">{t("load.price")}</th>
                  <th className="px-4 py-2.5 font-medium">{t("common.status")}</th>
                  <th className="px-4 py-2.5 text-right font-medium">{t("common.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {loads.data.items.map((load) => (
                  <tr key={load.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3">
                      <p className="font-mono text-xs">{load.reference_no}</p>
                      <p className="mt-0.5 max-w-[14rem] truncate text-xs text-muted-foreground">
                        {load.title}
                      </p>
                    </td>
                    <td className="px-4 py-3">
                      <p className="max-w-[16rem] truncate">
                        {load.pickup_address?.region_name ?? "—"} →{" "}
                        {load.delivery_address?.region_name ?? "—"}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {formatDate(load.pickup_from, locale)}
                      </p>
                    </td>
                    <td className="tnum px-4 py-3">{formatWeight(load.weight_kg, locale)}</td>
                    <td className="tnum px-4 py-3 font-medium">
                      {formatMoney(load.price_amount, load.price_currency, locale)}
                    </td>
                    <td className="px-4 py-3">
                      <Badge tone={TONE[load.status]}>{load.status.replace(/_/g, " ")}</Badge>
                      {load.reservations_count > 0 && load.status === "reserved" && (
                        <span className="ml-2 text-xs text-muted-foreground">
                          {load.reservations_count}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {/* Publishing is only offered where the backend allows it:
                          from draft or after expiry. */}
                      {(load.status === "draft" || load.status === "expired") && (
                        <Button
                          size="sm"
                          variant="accent"
                          loading={publish.isPending && publish.variables === load.id}
                          onClick={() => onPublish(load.id)}
                        >
                          {t("loads.publish")}
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>

          <Pagination
            meta={loads.data.meta}
            onPage={(page) => setFilters((prev) => ({ ...prev, page }))}
          />
        </>
      )}
    </div>
  );
}
