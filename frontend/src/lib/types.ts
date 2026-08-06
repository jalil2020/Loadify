/**
 * Response shapes mirroring the Go DTOs.
 *
 * Money is always an integer in minor units (tiyin) with an explicit currency —
 * never a float. Use formatMoney to render it.
 */

export type Locale = "uz" | "ru" | "en";

export type LoadStatus =
  | "draft"
  | "published"
  | "reserved"
  | "assigned"
  | "in_transit"
  | "delivered"
  | "completed"
  | "cancelled"
  | "expired";

export type TripStatus =
  | "created"
  | "en_route_to_pickup"
  | "at_pickup"
  | "loaded"
  | "in_transit"
  | "at_delivery"
  | "delivered"
  | "completed"
  | "cancelled"
  | "disputed";

export type BookingStatus =
  | "pending"
  | "offered"
  | "accepted"
  | "rejected"
  | "withdrawn"
  | "expired"
  | "cancelled";

export type VehicleType =
  | "tent"
  | "refrigerator"
  | "isothermal"
  | "flatbed"
  | "container"
  | "tanker"
  | "car_carrier"
  | "grain"
  | "dump"
  | "van"
  | "lowboy"
  | "other";

export interface Point {
  lat: number;
  lng: number;
}

export interface Address {
  id: string;
  region_id: string | null;
  region_name?: string;
  city_name?: string;
  line1: string;
  formatted: string;
  location: Point;
  contact_name: string | null;
  contact_phone: string | null;
}

export interface CurrentUser {
  id: string;
  phone: string;
  email: string | null;
  first_name: string;
  last_name: string;
  full_name: string;
  avatar_url: string | null;
  locale: Locale;
  status: string;
  phone_verified: boolean;
  roles: string[];
  permissions: string[];
  company_id: string | null;
  company_role: string | null;
  driver_id?: string | null;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  expires_at: string;
}

export interface AuthResponse {
  user: CurrentUser;
  tokens: TokenPair;
}

export interface LoadCompany {
  id: string;
  brand_name: string;
  logo_url: string | null;
  rating: number;
  rating_count: number;
  completed_trips: number;
  is_verified: boolean;
  /** Present only once the viewer has earned contact access. */
  phone?: string | null;
}

export interface BookingSummary {
  id: string;
  status: BookingStatus;
  initiated_by: "driver" | "company";
  price: number;
  currency: string;
  expires_at: string | null;
  created_at: string;
}

export interface Load {
  id: string;
  reference_no: string;
  company_id: string;
  title: string;
  description: string | null;
  cargo_type: string | null;
  weight_kg: number;
  volume_m3: number | null;
  packaging: string;
  pickup_address: Address | null;
  delivery_address: Address | null;
  pickup_from: string;
  pickup_to: string | null;
  delivery_from: string | null;
  distance_km: number | null;
  pickup_distance_km?: number | null;
  required_vehicle_type: VehicleType | null;
  requires_adr: boolean;
  requires_refrigeration: boolean;
  temp_min_c: number | null;
  temp_max_c: number | null;
  requires_tail_lift: boolean;
  price_amount: number;
  price_currency: string;
  price_type: string;
  price_per_km?: number | null;
  payment_terms: string;
  payment_method: string;
  status: LoadStatus;
  published_at: string | null;
  expires_at: string | null;
  views_count: number;
  reservations_count: number;
  is_featured: boolean;
  is_urgent: boolean;
  is_saved: boolean;
  company: LoadCompany | null;
  my_booking?: BookingSummary | null;
  created_at: string;
}

export interface VehicleSummary {
  id: string;
  type: VehicleType;
  make: string;
  model: string;
  plate_number: string;
  capacity_kg: number;
  volume_m3: number | null;
}

export interface BookingParty {
  user_id: string;
  full_name: string;
  avatar_url: string | null;
  rating: number;
  trips: number;
  phone?: string | null;
}

export interface LoadSummary {
  id: string;
  reference_no: string;
  title: string;
  status: LoadStatus;
  weight_kg: number;
  pickup_formatted: string;
  delivery_formatted: string;
  pickup_from: string;
  price_amount: number;
  price_currency: string;
  distance_km: number | null;
}

export interface Booking {
  id: string;
  load_id: string;
  load: LoadSummary | null;
  driver_id: string;
  driver: BookingParty | null;
  company_id: string;
  company: LoadCompany | null;
  vehicle: VehicleSummary | null;
  initiated_by: "driver" | "company";
  status: BookingStatus;
  offered_price: number | null;
  agreed_price: number | null;
  currency: string;
  message: string | null;
  response_message: string | null;
  expires_at: string | null;
  trip_id: string | null;
  created_at: string;
}

export interface Trip {
  id: string;
  trip_number: string;
  booking_id: string;
  load_id: string;
  load: LoadSummary | null;
  driver_id: string;
  driver: BookingParty | null;
  company: LoadCompany | null;
  vehicle: VehicleSummary | null;
  status: TripStatus;
  agreed_price: number;
  commission_amount: number;
  driver_payout: number;
  currency: string;
  distance_planned_km: number | null;
  distance_actual_km: number | null;
  eta_at: string | null;
  started_at: string | null;
  loaded_at: string | null;
  delivered_at: string | null;
  completed_at: string | null;
  current_location?: Point | null;
  can_rate: boolean;
  created_at: string;
}

export interface Driver {
  id: string;
  user_id: string;
  full_name: string;
  phone?: string;
  avatar_url: string | null;
  experience_years: number;
  languages: string[];
  status: "offline" | "online" | "busy";
  verification_status: string;
  rating: number;
  rating_count: number;
  completed_trips: number;
  acceptance_rate: number;
  current_location?: Point | null;
  distance_km?: number | null;
  vehicles?: VehicleSummary[];
}

export interface Region {
  id: string;
  code: string;
  name: string;
  name_uz: string;
  name_ru: string;
  name_en: string;
  center: Point | null;
}

export interface CargoType {
  id: string;
  code: string;
  name: string;
  icon: string | null;
  requires_adr: boolean;
  requires_temperature: boolean;
}

export interface VehicleTypeOption {
  code: VehicleType;
  name: string;
}

export interface DashboardStats {
  users: number;
  drivers: number;
  verified_drivers: number;
  online_drivers: number;
  companies: number;
  verified_companies: number;
  published_loads: number;
  active_trips: number;
  completed_trips: number;
  open_complaints: number;
  pending_documents: number;
  pending_refunds: number;
  gmv: number;
  commission_earned: number;
  new_users_today: number;
  loads_today: number;
  trips_today: number;
}

export interface Wallet {
  id: string;
  currency: string;
  balance: number;
  held_balance: number;
  available: number;
  status: string;
}
