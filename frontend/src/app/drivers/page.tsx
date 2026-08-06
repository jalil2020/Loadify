"use client";

import * as React from "react";
import { RequireAuth } from "@/components/guards";
import { useLocale } from "@/components/providers";
import { Pagination } from "@/components/pagination";
import {
  Badge,
  Card,
  EmptyState,
  ErrorState,
  Select,
  Skeleton,
} from "@/components/ui/primitives";
import { useDrivers, useVehicleTypes } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatPhone, formatWeight, initials } from "@/lib/format";

export default function DriversPage() {
  return (
    <RequireAuth permission="drivers.search">
      <DriversView />
    </RequireAuth>
  );
}

function DriversView() {
  const { t, locale } = useLocale();
  const [filters, setFilters] = React.useState<Record<string, unknown>>({ page: 1 });

  const vehicleTypes = useVehicleTypes();
  const drivers = useDrivers(filters);

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">{t("nav.drivers")}</h1>
        <div className="flex items-center gap-3">
          <Select
            className="w-44"
            value={(filters.vehicle_type as string) ?? ""}
            onChange={(e) =>
              setFilters((prev) => ({ ...prev, page: 1, vehicle_type: e.target.value || undefined }))
            }
          >
            <option value="">{t("filter.anyVehicle")}</option>
            {vehicleTypes.data?.map((v) => (
              <option key={v.code} value={v.code}>
                {v.name}
              </option>
            ))}
          </Select>
          <label className="flex cursor-pointer items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-input accent-[hsl(var(--accent))]"
              checked={Boolean(filters.online_only)}
              onChange={(e) =>
                setFilters((prev) => ({ ...prev, page: 1, online_only: e.target.checked || undefined }))
              }
            />
            online
          </label>
        </div>
      </header>

      {drivers.isError && (
        <ErrorState
          message={drivers.error instanceof ApiError ? drivers.error.message : t("common.error")}
          onRetry={() => drivers.refetch()}
          retryLabel={t("common.retry")}
        />
      )}

      {drivers.isPending && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-32 w-full" />
          ))}
        </div>
      )}

      {drivers.isSuccess && drivers.data.items.length === 0 && (
        <EmptyState title={t("board.empty")} hint={t("board.emptyHint")} />
      )}

      {drivers.isSuccess && drivers.data.items.length > 0 && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {drivers.data.items.map((driver) => (
              <Card key={driver.id} className="p-4">
                <div className="flex items-start gap-3">
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary text-sm font-semibold text-primary-foreground">
                    {initials(driver.full_name)}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="truncate font-medium">{driver.full_name}</p>
                      <Badge tone={driver.status === "online" ? "success" : driver.status === "busy" ? "warning" : "neutral"}>
                        {driver.status}
                      </Badge>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      ★ {driver.rating.toFixed(1)} · {driver.completed_trips} trips
                      {driver.distance_km != null && ` · ${driver.distance_km.toFixed(0)} km`}
                    </p>
                    {driver.phone && (
                      <a href={`tel:${driver.phone}`} className="text-xs text-primary hover:underline">
                        {formatPhone(driver.phone)}
                      </a>
                    )}
                    {driver.vehicles && driver.vehicles.length > 0 && (
                      <p className="mt-1.5 truncate text-xs text-muted-foreground">
                        {driver.vehicles[0]!.type} · {formatWeight(driver.vehicles[0]!.capacity_kg, locale)} ·{" "}
                        {driver.vehicles[0]!.plate_number}
                      </p>
                    )}
                  </div>
                </div>
              </Card>
            ))}
          </div>

          <Pagination
            meta={drivers.data.meta}
            onPage={(page) => setFilters((prev) => ({ ...prev, page }))}
          />
        </>
      )}
    </div>
  );
}
