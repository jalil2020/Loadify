"use client";

import * as React from "react";
import { RequireAuth } from "@/components/guards";
import { useAuth, useLocale } from "@/components/providers";
import { Pagination } from "@/components/pagination";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorState,
  Skeleton,
} from "@/components/ui/primitives";
import { useAdvanceTrip, useConfirmTrip, useTrips, type TripFilters } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatDistance, formatMoney, formatPhone } from "@/lib/format";
import type { Trip, TripStatus } from "@/lib/types";

/**
 * The driver-side progression, mirroring the backend state machine. Only these
 * transitions are a driver's to make: completion is the shipper confirming, and
 * cancellation has commercial consequences.
 */
const DRIVER_FLOW: TripStatus[] = [
  "created",
  "en_route_to_pickup",
  "at_pickup",
  "loaded",
  "in_transit",
  "at_delivery",
  "delivered",
];

const TONE: Record<TripStatus, "neutral" | "primary" | "success" | "warning" | "danger" | "accent"> = {
  created: "neutral",
  en_route_to_pickup: "primary",
  at_pickup: "primary",
  loaded: "warning",
  in_transit: "primary",
  at_delivery: "primary",
  delivered: "accent",
  completed: "success",
  cancelled: "danger",
  disputed: "danger",
};

function nextStatus(current: TripStatus): TripStatus | null {
  const index = DRIVER_FLOW.indexOf(current);
  if (index === -1 || index === DRIVER_FLOW.length - 1) return null;
  return DRIVER_FLOW[index + 1] ?? null;
}

export default function TripsPage() {
  return (
    <RequireAuth permission="trips.read">
      <TripsView />
    </RequireAuth>
  );
}

function TripsView() {
  const { t } = useLocale();
  const { user } = useAuth();
  const [filters, setFilters] = React.useState<TripFilters>({ page: 1 });
  const [error, setError] = React.useState<string | null>(null);

  const trips = useTrips(filters);
  const advance = useAdvanceTrip();
  const confirm = useConfirmTrip();

  const isDriver = Boolean(user?.driver_id);
  const isCompanySide = Boolean(user?.company_id);

  const run = async (fn: () => Promise<unknown>) => {
    setError(null);
    try {
      await fn();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"));
    }
  };

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">{t("nav.trips")}</h1>
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-input accent-[hsl(var(--accent))]"
            checked={filters.active_only ?? false}
            onChange={(e) => setFilters({ page: 1, active_only: e.target.checked || undefined })}
          />
          {t("common.status")}: active
        </label>
      </header>

      {error && <ErrorState message={error} retryLabel={t("common.retry")} />}

      {trips.isError && (
        <ErrorState
          message={trips.error instanceof ApiError ? trips.error.message : t("common.error")}
          onRetry={() => trips.refetch()}
          retryLabel={t("common.retry")}
        />
      )}

      {trips.isPending && (
        <div className="space-y-3">
          {Array.from({ length: 3 }, (_, i) => (
            <Skeleton key={i} className="h-36 w-full" />
          ))}
        </div>
      )}

      {trips.isSuccess && trips.data.items.length === 0 && <EmptyState title={t("trip.empty")} />}

      {trips.isSuccess && trips.data.items.length > 0 && (
        <>
          <div className="space-y-3">
            {trips.data.items.map((trip) => (
              <TripCard
                key={trip.id}
                trip={trip}
                isDriver={isDriver}
                isCompanySide={isCompanySide}
                advancing={advance.isPending && advance.variables?.tripId === trip.id}
                confirming={confirm.isPending && confirm.variables === trip.id}
                onAdvance={(status) => run(() => advance.mutateAsync({ tripId: trip.id, status }))}
                onConfirm={() => run(() => confirm.mutateAsync(trip.id))}
              />
            ))}
          </div>

          <Pagination
            meta={trips.data.meta}
            onPage={(page) => setFilters((prev) => ({ ...prev, page }))}
          />
        </>
      )}
    </div>
  );
}

function TripCard({
  trip,
  isDriver,
  isCompanySide,
  advancing,
  confirming,
  onAdvance,
  onConfirm,
}: {
  trip: Trip;
  isDriver: boolean;
  isCompanySide: boolean;
  advancing: boolean;
  confirming: boolean;
  onAdvance: (status: TripStatus) => void;
  onConfirm: () => void;
}) {
  const { t, locale } = useLocale();
  const next = nextStatus(trip.status);
  const stageIndex = DRIVER_FLOW.indexOf(trip.status);
  const finished = trip.status === "completed" || trip.status === "cancelled";

  return (
    <Card className="p-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          <div className="mb-1.5 flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs text-muted-foreground">{trip.trip_number}</span>
            <Badge tone={TONE[trip.status]}>{trip.status.replace(/_/g, " ")}</Badge>
          </div>

          <h3 className="truncate font-medium">{trip.load?.title ?? "—"}</h3>
          <p className="mt-0.5 truncate text-sm text-muted-foreground">
            {trip.load?.pickup_formatted} → {trip.load?.delivery_formatted}
            {trip.load?.distance_km != null && ` · ${formatDistance(trip.load.distance_km, locale)}`}
          </p>

          {/* Progress rail: seven fixed stages, so a driver can see at a glance
              how far along the shipment is. */}
          {!finished && stageIndex >= 0 && (
            <ol className="mt-3 flex items-center gap-1" aria-label="Trip progress">
              {DRIVER_FLOW.map((stage, i) => (
                <li
                  key={stage}
                  title={stage.replace(/_/g, " ")}
                  className={`h-1.5 flex-1 rounded-full ${
                    i <= stageIndex ? "bg-accent" : "bg-muted"
                  }`}
                />
              ))}
            </ol>
          )}

          <div className="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-sm">
            {trip.driver && (
              <span>
                <span className="text-muted-foreground">Driver: </span>
                {trip.driver.full_name}
                {trip.driver.phone && (
                  <a href={`tel:${trip.driver.phone}`} className="ml-2 text-primary hover:underline">
                    {formatPhone(trip.driver.phone)}
                  </a>
                )}
              </span>
            )}
            {trip.vehicle && (
              <span className="text-muted-foreground">
                {trip.vehicle.plate_number} · {trip.vehicle.make} {trip.vehicle.model}
              </span>
            )}
          </div>
        </div>

        <div className="shrink-0 lg:w-56 lg:text-right">
          <p className="tnum text-lg font-semibold">
            {formatMoney(trip.agreed_price, trip.currency, locale)}
          </p>
          <p className="tnum text-xs text-muted-foreground">
            {t("trip.commission")} {formatMoney(trip.commission_amount, trip.currency, locale)}
          </p>
          <p className="tnum text-xs font-medium text-accent">
            {t("trip.payout")} {formatMoney(trip.driver_payout, trip.currency, locale)}
          </p>

          <div className="mt-3 flex flex-col gap-2 lg:items-end">
            {isDriver && next && (
              <Button size="sm" variant="accent" loading={advancing} onClick={() => onAdvance(next)}>
                {t("trip.advance")}: {next.replace(/_/g, " ")}
              </Button>
            )}
            {isCompanySide && trip.status === "delivered" && (
              <Button size="sm" loading={confirming} onClick={onConfirm}>
                {t("trip.confirm")}
              </Button>
            )}
          </div>
        </div>
      </div>
    </Card>
  );
}
