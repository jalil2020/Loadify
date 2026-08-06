"use client";

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import { api, type Envelope, type PageMeta } from "@/lib/api";
import type {
  Booking,
  BookingStatus,
  CargoType,
  CurrentUser,
  DashboardStats,
  Driver,
  Load,
  Region,
  Trip,
  TripStatus,
  VehicleTypeOption,
  Wallet,
} from "@/lib/types";

/**
 * Query keys are declared once so an invalidation cannot miss a cache entry
 * through a typo — the commonest way stale data survives a mutation.
 */
export const keys = {
  me: ["me"] as const,
  regions: (country: string) => ["regions", country] as const,
  vehicleTypes: ["vehicle-types"] as const,
  cargoTypes: ["cargo-types"] as const,
  loads: (filters: unknown) => ["loads", filters] as const,
  load: (id: string) => ["load", id] as const,
  myLoads: (filters: unknown) => ["my-loads", filters] as const,
  bookings: (filters: unknown) => ["bookings", filters] as const,
  trips: (filters: unknown) => ["trips", filters] as const,
  trip: (id: string) => ["trip", id] as const,
  drivers: (filters: unknown) => ["drivers", filters] as const,
  wallet: ["wallet"] as const,
  dashboard: ["admin", "dashboard"] as const,
  corridors: ["admin", "corridors"] as const,
};

export interface Paged<T> {
  items: T[];
  meta: PageMeta;
}

const EMPTY_META: PageMeta = { page: 1, per_page: 20, total: 0, total_pages: 0 };

function paged<T>(res: Envelope<T[]>): Paged<T> {
  return { items: res.data ?? [], meta: res.meta ?? EMPTY_META };
}

/* ── Reference data ───────────────────────────────────────────────── */

// Reference vocabularies change only on deploy, so they are cached for the
// session rather than refetched per page.
const REFERENCE_OPTIONS = { staleTime: 60 * 60_000, gcTime: 60 * 60_000 } as const;

export function useRegions(country = "UZ") {
  return useQuery({
    queryKey: keys.regions(country),
    queryFn: async () => (await api.get<Region[]>("/reference/regions", { country })).data,
    ...REFERENCE_OPTIONS,
  });
}

export function useVehicleTypes() {
  return useQuery({
    queryKey: keys.vehicleTypes,
    queryFn: async () => (await api.get<VehicleTypeOption[]>("/reference/vehicle-types")).data,
    ...REFERENCE_OPTIONS,
  });
}

export function useCargoTypes() {
  return useQuery({
    queryKey: keys.cargoTypes,
    queryFn: async () => (await api.get<CargoType[]>("/reference/cargo-types")).data,
    ...REFERENCE_OPTIONS,
  });
}

/* ── Loads ────────────────────────────────────────────────────────── */

export interface LoadFilters {
  page?: number;
  per_page?: number;
  sort?: string;
  pickup_region_id?: string;
  delivery_region_id?: string;
  vehicle_type?: string;
  min_weight_kg?: number;
  max_weight_kg?: number;
  min_price?: number;
  max_price?: number;
  fits_my_vehicle?: boolean;
  status?: string;
}

export function useLoads(filters: LoadFilters, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: keys.loads(filters),
    queryFn: async () => paged(await api.get<Load[]>("/loads", filters)),
    enabled: options?.enabled ?? true,
  });
}

export function useMyLoads(filters: LoadFilters) {
  return useQuery({
    queryKey: keys.myLoads(filters),
    queryFn: async () => paged(await api.get<Load[]>("/loads/my", filters)),
  });
}

export function useLoad(id: string | undefined) {
  return useQuery({
    queryKey: keys.load(id ?? ""),
    queryFn: async () => (await api.get<Load>(`/loads/${id}`)).data,
    enabled: Boolean(id),
  });
}

/* ── Bookings ─────────────────────────────────────────────────────── */

export interface BookingFilters {
  page?: number;
  status?: BookingStatus[];
  open_only?: boolean;
}

export function useBookings(filters: BookingFilters, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: keys.bookings(filters),
    queryFn: async () => paged(await api.get<Booking[]>("/bookings", filters)),
    enabled: options?.enabled ?? true,
  });
}

/* ── Trips ────────────────────────────────────────────────────────── */

export interface TripFilters {
  page?: number;
  status?: TripStatus[];
  active_only?: boolean;
}

export function useTrips(filters: TripFilters, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: keys.trips(filters),
    queryFn: async () => paged(await api.get<Trip[]>("/trips", filters)),
    enabled: options?.enabled ?? true,
  });
}

export function useTrip(id: string | undefined) {
  return useQuery({
    queryKey: keys.trip(id ?? ""),
    queryFn: async () => (await api.get<Trip>(`/trips/${id}`)).data,
    enabled: Boolean(id),
  });
}

/* ── Drivers, wallet, admin ───────────────────────────────────────── */

export function useDrivers(filters: Record<string, unknown>, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: keys.drivers(filters),
    queryFn: async () => paged(await api.get<Driver[]>("/drivers", filters)),
    enabled: options?.enabled ?? true,
  });
}

export function useWallet(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: keys.wallet,
    queryFn: async () => (await api.get<Wallet>("/wallet")).data,
    enabled: options?.enabled ?? true,
  });
}

export interface Corridor {
  pickup_region: string;
  delivery_region: string;
  loads: number;
  trips: number;
  avg_price: number;
  fill_rate: number;
}

export function useDashboard(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: keys.dashboard,
    queryFn: async () => (await api.get<DashboardStats>("/admin/dashboard")).data,
    enabled: options?.enabled ?? true,
  });
}

export function useCorridors(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: keys.corridors,
    queryFn: async () => (await api.get<Corridor[]>("/admin/analytics/corridors")).data,
    enabled: options?.enabled ?? true,
  });
}

export function useMe(options?: Partial<UseQueryOptions<CurrentUser>>) {
  return useQuery({
    queryKey: keys.me,
    queryFn: async () => (await api.get<CurrentUser>("/auth/me")).data,
    ...options,
  });
}

/* ── Mutations ────────────────────────────────────────────────────── */

/**
 * Reserving a load changes the board, the caller's bookings and that load's
 * detail, so all three are invalidated. Under-invalidating here is what makes a
 * UI show a load as available seconds after the user took it.
 */
export function useReserveLoad() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ loadId, message }: { loadId: string; message?: string }) =>
      (await api.post<Booking>(`/loads/${loadId}/reserve`, { message })).data,
    onSuccess: (_booking, { loadId }) => {
      qc.invalidateQueries({ queryKey: ["loads"] });
      qc.invalidateQueries({ queryKey: keys.load(loadId) });
      qc.invalidateQueries({ queryKey: ["bookings"] });
    },
  });
}

export function useRespondBooking() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      bookingId,
      accept,
      agreedPrice,
    }: {
      bookingId: string;
      accept: boolean;
      agreedPrice?: number;
    }) =>
      (
        await api.post<Booking>(`/bookings/${bookingId}/respond`, {
          accept,
          agreed_price: agreedPrice,
        })
      ).data,
    onSuccess: () => {
      // Accepting creates a trip and moves the load, so every list shifts.
      qc.invalidateQueries({ queryKey: ["bookings"] });
      qc.invalidateQueries({ queryKey: ["trips"] });
      qc.invalidateQueries({ queryKey: ["loads"] });
      qc.invalidateQueries({ queryKey: ["my-loads"] });
    },
  });
}

export function useAdvanceTrip() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ tripId, status }: { tripId: string; status: TripStatus }) =>
      (await api.post<Trip>(`/trips/${tripId}/advance`, { status })).data,
    onSuccess: (_trip, { tripId }) => {
      qc.invalidateQueries({ queryKey: ["trips"] });
      qc.invalidateQueries({ queryKey: keys.trip(tripId) });
    },
  });
}

export function useConfirmTrip() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (tripId: string) => (await api.post<Trip>(`/trips/${tripId}/confirm`)).data,
    onSuccess: (_trip, tripId) => {
      qc.invalidateQueries({ queryKey: ["trips"] });
      qc.invalidateQueries({ queryKey: keys.trip(tripId) });
      // Completion releases escrow and takes commission.
      qc.invalidateQueries({ queryKey: keys.wallet });
      qc.invalidateQueries({ queryKey: ["my-loads"] });
    },
  });
}

export function usePublishLoad() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (loadId: string) => (await api.post<Load>(`/loads/${loadId}/publish`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["my-loads"] });
      qc.invalidateQueries({ queryKey: ["loads"] });
    },
  });
}

export function useToggleSaveLoad() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ loadId, saved }: { loadId: string; saved: boolean }) => {
      if (saved) await api.delete(`/loads/${loadId}/save`);
      else await api.post(`/loads/${loadId}/save`);
      return !saved;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["loads"] }),
  });
}
