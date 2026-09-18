// Shapes the API returns. They mirror backend/internal/dto and
// backend/docs/openapi.yaml; the API is the source of truth.

export type UUID = string;

export interface PageMeta {
  page: number;
  per_page: number;
  total: number;
  total_pages: number;
}

export interface Page<T> {
  items: T[];
  meta: PageMeta;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_at: string;
}

export interface User {
  id: UUID;
  phone: string;
  first_name: string;
  last_name: string;
  full_name: string;
  locale: string;
  status: string;
  roles: string[];
  permissions: string[];
  company_id?: UUID;
  company_role?: string;
}

export interface AuthResponse {
  user: User;
  tokens: TokenPair;
}

export interface Company {
  id: UUID;
  legal_name: string;
  brand_name: string;
  phone?: string;
  status: string;
}

// ── reference data ──────────────────────────────────────────────────
export interface Country {
  id: UUID;
  code: string;
  name: string;
  flag_emoji?: string | null;
}

export interface Region {
  id: UUID;
  code: string;
  name: string;
}

export interface VehicleType {
  code: string;
  name: string;
}

export interface CargoType {
  id: UUID;
  code: string;
  name: string;
}

// ── adverts ─────────────────────────────────────────────────────────
export type AdStatus = "draft" | "ready" | "publishing" | "published" | "archived";

export interface PlaceRef {
  city_id?: UUID | null;
  region_id?: UUID | null;
  country?: string | null;
  text?: string;
}

export interface LoadAd {
  id: UUID;
  status: AdStatus;
  raw_text?: string;
  origin: PlaceRef;
  destination: PlaceRef;
  cargo_type_id?: UUID | null;
  cargo_text?: string | null;
  weight_kg?: number | null;
  volume_m3?: number | null;
  vehicle_type?: string | null;
  loading_date?: string | null;
  loading_date_text?: string | null;
  price_amount?: number | null;
  price_currency?: string | null;
  price_negotiable: boolean;
  payment_terms?: string | null;
  payment_method?: string | null;
  contact_phone?: string | null;
  contact_name?: string | null;
  notes?: string | null;
  missing: string[];
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

/** What the create and update endpoints accept. */
export interface SaveLoadAd {
  raw_text?: string;
  origin_city_id?: UUID | null;
  origin_region_id?: UUID | null;
  origin_country?: string | null;
  origin_text?: string;
  destination_city_id?: UUID | null;
  destination_region_id?: UUID | null;
  destination_country?: string | null;
  destination_text?: string;
  cargo_type_id?: UUID | null;
  cargo_text?: string | null;
  weight_kg?: number | null;
  volume_m3?: number | null;
  vehicle_type?: string | null;
  loading_date?: string | null;
  loading_date_text?: string | null;
  price_amount?: number | null;
  price_currency?: string | null;
  price_negotiable?: boolean;
  payment_terms?: string | null;
  payment_method?: string | null;
  contact_phone?: string | null;
  contact_name?: string | null;
  notes?: string | null;
}

// ── distribution ────────────────────────────────────────────────────
export interface GroupDecision {
  group_id: UUID;
  title: string;
  score?: number;
  reasons?: string[];
  reason?: string;
  retry_after_seconds?: number;
}

export interface PublishPreview {
  account_id: UUID;
  candidates: number;
  approved: GroupDecision[];
  skipped: GroupDecision[];
  message_preview: string;
  preview_token: string;
  expires_at: string;
  posts_left: number | null;
}

export type JobStatus =
  | "pending"
  | "processing"
  | "sent"
  | "failed"
  | "skipped"
  | "rate_limited"
  | "cancelled";

export interface PostingJob {
  id: UUID;
  group_id: UUID;
  group_title?: string;
  status: JobStatus;
  skip_reason?: string;
  attempt: number;
  error_code?: string;
  next_try_at?: string | null;
  sent_at?: string | null;
  created_at: string;
}

export interface PublishResult {
  run_id: UUID;
  load_ad_id: UUID;
  account_id: UUID;
  queued: number;
  skipped: number;
  jobs: PostingJob[];
}

// ── telegram ────────────────────────────────────────────────────────
export type AccountStatus = "connecting" | "active" | "paused" | "restricted" | "revoked" | "disabled";

export interface TelegramAccount {
  id: UUID;
  user_id: UUID;
  mine: boolean;
  phone: string;
  label?: string;
  status: AccountStatus;
  status_reason?: string | null;
  paused_until?: string | null;
  consent_at?: string | null;
  last_used_at?: string | null;
  created_at: string;
}

export type LoginStep = "code" | "password" | "connected";

export interface TelegramLoginStep {
  account_id: UUID;
  step: LoginStep;
}

export interface TelegramDialog {
  peer_id: string;
  title: string;
  username?: string | null;
  kind: string;
  member_count?: number | null;
  slowmode_enabled: boolean;
  can_send: boolean;
  group_id?: UUID | null;
}

export interface GroupRoute {
  origin_country?: string | null;
  origin_region_id?: UUID | null;
  destination_country?: string | null;
  destination_region_id?: UUID | null;
}

export interface GroupRules {
  cargo_type_codes: string[];
  vehicle_types: string[];
  min_weight_kg?: number | null;
  max_weight_kg?: number | null;
  hourly_limit: number;
  daily_limit: number;
  cooldown_minutes: number;
  quiet_from_hour?: number | null;
  quiet_to_hour?: number | null;
  notes?: string | null;
}

export interface TelegramGroup {
  id: UUID;
  account_id: UUID;
  peer_id: string;
  title: string;
  username?: string | null;
  kind: string;
  language: string;
  member_count?: number | null;
  slowmode_seconds: number;
  can_send: boolean;
  posting_enabled: boolean;
  active: boolean;
  inactive_reason?: string | null;
  last_posted_at?: string | null;
  rules?: GroupRules;
  routes?: GroupRoute[];
}

export interface SaveGroupRules {
  language?: string;
  posting_enabled?: boolean;
  cargo_type_codes: string[];
  vehicle_types: string[];
  min_weight_kg?: number | null;
  max_weight_kg?: number | null;
  hourly_limit: number;
  daily_limit: number;
  cooldown_minutes: number;
  quiet_from_hour?: number | null;
  quiet_to_hour?: number | null;
  notes?: string | null;
  routes: GroupRoute[];
}

// ── plan and dashboard ──────────────────────────────────────────────
export type QuotaMetric = "loads" | "posts" | "ai_requests" | "tg_accounts" | "tg_groups";

export interface QuotaItem {
  metric: QuotaMetric;
  limit: number | null;
  used: number;
  remaining: number | null;
  resets_at?: string;
}

export interface Usage {
  period_start: string;
  period_end: string;
  items: QuotaItem[];
}

export interface PostCounts {
  total: number;
  queued: number;
  sent: number;
  failed: number;
  skipped: number;
  cancelled: number;
}

export interface Dashboard {
  scope: "mine" | "company";
  day: string;
  timezone: string;
  loads: { today: number; month: number };
  posts: {
    today: PostCounts;
    month: PostCounts;
    success_rate: number | null;
    last_sent_at: string | null;
  };
  guard_today: Record<string, number>;
  accounts: { total: number; by_status: Record<string, number> };
  groups: { connected: number; postable: number };
  ai: {
    enabled: boolean;
    requests_today: number;
    requests_month: number;
    cache_hits_month: number;
    cost_month_usd: number;
  };
  quota: QuotaItem[];
  recent_problems: {
    job_id: UUID;
    group_id: UUID;
    group_title: string;
    status: JobStatus;
    reason: string;
    at: string;
  }[];
}

export interface Notification {
  id: UUID;
  type: string;
  title: string;
  body: string;
  data?: Record<string, unknown>;
  priority: string;
  action_url?: string | null;
  read_at?: string | null;
  created_at: string;
}
