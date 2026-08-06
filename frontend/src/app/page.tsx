"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useAuth, useLocale } from "@/components/providers";
import { LoadCard, LoadCardSkeleton } from "@/components/load-card";
import { Pagination } from "@/components/pagination";
import {
  Button,
  Card,
  EmptyState,
  ErrorState,
  Field,
  Input,
  Select,
} from "@/components/ui/primitives";
import { useLoads, useRegions, useReserveLoad, useVehicleTypes, type LoadFilters } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatNumber } from "@/lib/format";
import type { Load } from "@/lib/types";

const EMPTY_FILTERS: LoadFilters = { page: 1, per_page: 20 };

export default function LoadBoardPage() {
  const { t, locale } = useLocale();
  const { user, can } = useAuth();
  const router = useRouter();

  const [filters, setFilters] = React.useState<LoadFilters>(EMPTY_FILTERS);
  // A rejected reservation and a successful one are not the same news, so the
  // banner carries its tone: a failure is styled as a failure and announced
  // assertively, rather than sharing the neutral confirmation styling.
  const [notice, setNotice] = React.useState<{ tone: "ok" | "error"; text: string } | null>(null);

  const regions = useRegions();
  const vehicleTypes = useVehicleTypes();
  const loads = useLoads(filters);
  const reserve = useReserveLoad();

  // Changing a filter must reset to the first page, or the user lands on an
  // empty page 4 of a now-shorter result set.
  const update = (patch: Partial<LoadFilters>) =>
    setFilters((prev) => ({ ...prev, ...patch, page: 1 }));

  const canReserve = can("bookings.create");

  const onReserve = async (load: Load) => {
    if (!user) {
      router.push(`/login?next=${encodeURIComponent("/")}`);
      return;
    }
    setNotice(null);
    try {
      await reserve.mutateAsync({ loadId: load.id });
      setNotice({ tone: "ok", text: `${load.reference_no}: ${t("board.reserved")}` });
    } catch (error) {
      setNotice({
        tone: "error",
        text: error instanceof ApiError ? error.message : t("common.error"),
      });
    }
  };

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("board.title")}</h1>
          <p className="text-sm text-muted-foreground">
            {loads.isSuccess
              ? `${formatNumber(loads.data.meta.total, locale)} ${t("board.subtitle")}`
              : t("common.loading")}
          </p>
        </div>

        {can("loads.create") && (
          <Button onClick={() => router.push("/my-loads")}>{t("loads.mine")}</Button>
        )}
      </header>

      <Card className="p-4">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <Field label={t("filter.from")}>
            <Select
              value={filters.pickup_region_id ?? ""}
              onChange={(e) => update({ pickup_region_id: e.target.value || undefined })}
            >
              <option value="">{t("filter.anyRegion")}</option>
              {regions.data?.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </Select>
          </Field>

          <Field label={t("filter.to")}>
            <Select
              value={filters.delivery_region_id ?? ""}
              onChange={(e) => update({ delivery_region_id: e.target.value || undefined })}
            >
              <option value="">{t("filter.anyRegion")}</option>
              {regions.data?.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </Select>
          </Field>

          <Field label={t("filter.vehicleType")}>
            <Select
              value={filters.vehicle_type ?? ""}
              onChange={(e) => update({ vehicle_type: e.target.value || undefined })}
            >
              <option value="">{t("filter.anyVehicle")}</option>
              {vehicleTypes.data?.map((v) => (
                <option key={v.code} value={v.code}>
                  {v.name}
                </option>
              ))}
            </Select>
          </Field>

          <Field label={t("filter.minWeight")}>
            <Input
              type="number"
              min={0}
              inputMode="numeric"
              value={filters.min_weight_kg ?? ""}
              onChange={(e) =>
                update({ min_weight_kg: e.target.value ? Number(e.target.value) : undefined })
              }
            />
          </Field>

          <Field label={t("filter.maxWeight")}>
            <Input
              type="number"
              min={0}
              inputMode="numeric"
              value={filters.max_weight_kg ?? ""}
              onChange={(e) =>
                update({ max_weight_kg: e.target.value ? Number(e.target.value) : undefined })
              }
            />
          </Field>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-4">
          {/* Only meaningful for a driver: it filters against their own truck. */}
          {user?.driver_id && (
            <label className="flex cursor-pointer items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border-input accent-[hsl(var(--accent))]"
                checked={filters.fits_my_vehicle ?? false}
                onChange={(e) => update({ fits_my_vehicle: e.target.checked || undefined })}
              />
              {t("board.fitsMyVehicle")}
            </label>
          )}

          <Button
            variant="ghost"
            size="sm"
            className="ml-auto"
            onClick={() => setFilters(EMPTY_FILTERS)}
          >
            {t("filter.reset")}
          </Button>
        </div>
      </Card>

      {notice && (
        <div
          role={notice.tone === "error" ? "alert" : "status"}
          className={
            notice.tone === "error"
              ? "rounded-md border border-destructive/30 bg-destructive/10 px-4 py-2 text-sm text-destructive"
              : "rounded-md border border-accent/30 bg-accent/10 px-4 py-2 text-sm"
          }
        >
          {notice.text}
        </div>
      )}

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
            <LoadCardSkeleton key={i} />
          ))}
        </div>
      )}

      {loads.isSuccess && loads.data.items.length === 0 && (
        <EmptyState
          title={t("board.empty")}
          hint={t("board.emptyHint")}
          action={
            <Button variant="outline" onClick={() => setFilters(EMPTY_FILTERS)}>
              {t("filter.reset")}
            </Button>
          }
        />
      )}

      {loads.isSuccess && loads.data.items.length > 0 && (
        <>
          <div className="space-y-3">
            {loads.data.items.map((load) => (
              <LoadCard
                key={load.id}
                load={load}
                canReserve={canReserve}
                reserving={reserve.isPending && reserve.variables?.loadId === load.id}
                onReserve={onReserve}
              />
            ))}
          </div>

          <Pagination
            meta={loads.data.meta}
            onPage={(page) => setFilters((prev) => ({ ...prev, page }))}
          />
        </>
      )}
    </div>
  );
}
