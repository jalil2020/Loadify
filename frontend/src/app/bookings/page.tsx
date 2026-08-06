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
import { useBookings, useRespondBooking, type BookingFilters } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatDateTime, formatMoney, formatPhone, formatRelative } from "@/lib/format";
import type { Booking, BookingStatus } from "@/lib/types";

const TONE: Record<BookingStatus, "neutral" | "primary" | "success" | "warning" | "danger" | "accent"> = {
  pending: "warning",
  offered: "warning",
  accepted: "success",
  rejected: "danger",
  withdrawn: "neutral",
  expired: "neutral",
  cancelled: "danger",
};

export default function BookingsPage() {
  return (
    <RequireAuth permission="bookings.read">
      <BookingsView />
    </RequireAuth>
  );
}

function BookingsView() {
  const { t, locale } = useLocale();
  const { user, can } = useAuth();
  const [filters, setFilters] = React.useState<BookingFilters>({ page: 1 });
  const [error, setError] = React.useState<string | null>(null);

  const bookings = useBookings(filters);
  const respond = useRespondBooking();

  const isCompanySide = Boolean(user?.company_id);

  const onRespond = async (booking: Booking, accept: boolean) => {
    setError(null);
    try {
      await respond.mutateAsync({ bookingId: booking.id, accept });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.error"));
    }
  };

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">{t("nav.bookings")}</h1>
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-input accent-[hsl(var(--accent))]"
            checked={filters.open_only ?? false}
            onChange={(e) =>
              setFilters({ page: 1, open_only: e.target.checked || undefined })
            }
          />
          {t("booking.pending")}
        </label>
      </header>

      {error && (
        <ErrorState message={error} retryLabel={t("common.retry")} />
      )}

      {bookings.isError && (
        <ErrorState
          message={bookings.error instanceof ApiError ? bookings.error.message : t("common.error")}
          onRetry={() => bookings.refetch()}
          retryLabel={t("common.retry")}
        />
      )}

      {bookings.isPending && (
        <div className="space-y-3">
          {Array.from({ length: 3 }, (_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
      )}

      {bookings.isSuccess && bookings.data.items.length === 0 && (
        <EmptyState title={t("booking.empty")} />
      )}

      {bookings.isSuccess && bookings.data.items.length > 0 && (
        <>
          <div className="space-y-3">
            {bookings.data.items.map((booking) => {
              // Only the side that did NOT initiate may answer, which is
              // exactly what the backend enforces.
              const canAnswer =
                can("bookings.respond") &&
                (booking.status === "pending" || booking.status === "offered") &&
                (isCompanySide
                  ? booking.initiated_by === "driver"
                  : booking.initiated_by === "company");

              const counterparty = isCompanySide ? booking.driver : booking.company;
              const counterpartyName = isCompanySide
                ? booking.driver?.full_name
                : booking.company?.brand_name;
              const phone = isCompanySide ? booking.driver?.phone : booking.company?.phone;

              return (
                <Card key={booking.id} className="p-5">
                  <div className="flex flex-col gap-4 lg:flex-row lg:items-center">
                    <div className="min-w-0 flex-1">
                      <div className="mb-1.5 flex flex-wrap items-center gap-2">
                        <Badge tone={TONE[booking.status]}>{booking.status}</Badge>
                        <span className="font-mono text-xs text-muted-foreground">
                          {booking.load?.reference_no}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {booking.initiated_by === "driver" ? "↑ driver" : "↓ company"}
                        </span>
                      </div>
                      <h3 className="truncate font-medium">{booking.load?.title ?? "—"}</h3>
                      <p className="mt-0.5 truncate text-sm text-muted-foreground">
                        {booking.load?.pickup_formatted} → {booking.load?.delivery_formatted}
                      </p>
                      {booking.message && (
                        <p className="mt-2 rounded-md bg-muted px-3 py-1.5 text-sm">
                          “{booking.message}”
                        </p>
                      )}
                    </div>

                    <div className="shrink-0 text-sm lg:w-56">
                      <p className="font-medium">{counterpartyName ?? "—"}</p>
                      {counterparty && "rating" in counterparty && (
                        <p className="text-xs text-muted-foreground">
                          ★ {counterparty.rating.toFixed(1)}
                        </p>
                      )}
                      {phone && (
                        <a href={`tel:${phone}`} className="text-xs text-primary hover:underline">
                          {formatPhone(phone)}
                        </a>
                      )}
                      {booking.vehicle && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          {booking.vehicle.make} {booking.vehicle.model} · {booking.vehicle.plate_number}
                        </p>
                      )}
                    </div>

                    <div className="shrink-0 lg:w-48 lg:text-right">
                      <p className="tnum text-lg font-semibold">
                        {formatMoney(
                          booking.agreed_price ?? booking.offered_price ?? 0,
                          booking.currency,
                          locale,
                        )}
                      </p>
                      {booking.expires_at && booking.status !== "accepted" && (
                        <p className="text-xs text-muted-foreground">
                          {formatRelative(booking.expires_at, locale)}
                        </p>
                      )}
                      {!booking.expires_at && (
                        <p className="text-xs text-muted-foreground">
                          {formatDateTime(booking.created_at, locale)}
                        </p>
                      )}

                      {canAnswer && (
                        <div className="mt-3 flex gap-2 lg:justify-end">
                          <Button
                            variant="accent"
                            size="sm"
                            loading={respond.isPending && respond.variables?.bookingId === booking.id}
                            onClick={() => onRespond(booking, true)}
                          >
                            {t("booking.accept")}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => onRespond(booking, false)}
                          >
                            {t("booking.reject")}
                          </Button>
                        </div>
                      )}
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>

          <Pagination
            meta={bookings.data.meta}
            onPage={(page) => setFilters((prev) => ({ ...prev, page }))}
          />
        </>
      )}
    </div>
  );
}
