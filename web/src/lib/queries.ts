import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useI18n } from "../i18n";
import { api } from "./api";
import type {
  CargoType,
  Country,
  Dashboard,
  LoadAd,
  Notification,
  PostingJob,
  PublishPreview,
  PublishResult,
  Region,
  SaveGroupRules,
  SaveLoadAd,
  TelegramAccount,
  TelegramDialog,
  TelegramGroup,
  TelegramLoginStep,
  Usage,
  UUID,
  VehicleType,
} from "./types";

export const keys = {
  dashboard: (scope: string) => ["dashboard", scope] as const,
  usage: ["usage"] as const,
  ads: ["ads"] as const,
  adList: (f: AdFilters) => ["ads", "list", f] as const,
  ad: (id: UUID) => ["ads", "detail", id] as const,
  adPosts: (id: UUID) => ["ads", "posts", id] as const,
  history: (f: HistoryFilters) => ["history", f] as const,
  accounts: (company: boolean) => ["telegram", "accounts", company] as const,
  dialogs: (accountId: UUID) => ["telegram", "dialogs", accountId] as const,
  groups: ["telegram", "groups"] as const,
  notifications: (unread: boolean, page: number) => ["notifications", unread, page] as const,
  unread: ["notifications", "unread"] as const,
  ref: (name: string, ...args: (string | undefined)[]) => ["ref", name, ...args] as const,
};

// ── reference data ──────────────────────────────────────────────────
// Localised on the server, so the language is part of every key.

const REF_STALE = 60 * 60 * 1000;

export function useCountries() {
  const { lang } = useI18n();
  return useQuery({
    queryKey: keys.ref("countries", lang),
    queryFn: () => api.get<Country[]>("/reference/countries"),
    staleTime: REF_STALE,
  });
}

export function useRegions(country: string | null | undefined) {
  const { lang } = useI18n();
  return useQuery({
    queryKey: keys.ref("regions", country ?? "", lang),
    queryFn: () => api.get<Region[]>("/reference/regions", { country }),
    enabled: Boolean(country),
    staleTime: REF_STALE,
  });
}

export function useVehicleTypes() {
  const { lang } = useI18n();
  return useQuery({
    queryKey: keys.ref("vehicle-types", lang),
    queryFn: () => api.get<VehicleType[]>("/reference/vehicle-types"),
    staleTime: REF_STALE,
  });
}

export function useCargoTypes() {
  const { lang } = useI18n();
  return useQuery({
    queryKey: keys.ref("cargo-types", lang),
    queryFn: () => api.get<CargoType[]>("/reference/cargo-types"),
    staleTime: REF_STALE,
  });
}

// ── dashboard and plan ──────────────────────────────────────────────

export function useDashboard(scope?: "mine" | "company") {
  return useQuery({
    queryKey: keys.dashboard(scope ?? "default"),
    queryFn: () => api.get<Dashboard>("/dashboard", { scope }),
    refetchInterval: 60_000,
  });
}

export function useUsage() {
  return useQuery({ queryKey: keys.usage, queryFn: () => api.get<Usage>("/subscriptions/usage") });
}

// ── adverts ─────────────────────────────────────────────────────────

export interface AdFilters {
  status?: string;
  search?: string;
  mine?: boolean;
  page: number;
}

export function useAds(filters: AdFilters) {
  return useQuery({
    queryKey: keys.adList(filters),
    queryFn: () =>
      api.page<LoadAd>("/load-ads", {
        status: filters.status,
        search: filters.search,
        mine: filters.mine || undefined,
        page: filters.page,
        per_page: 20,
      }),
    placeholderData: keepPreviousData,
  });
}

export function useAd(id: UUID | undefined) {
  return useQuery({
    queryKey: keys.ad(id ?? ""),
    queryFn: () => api.get<LoadAd>(`/load-ads/${id}`),
    enabled: Boolean(id),
  });
}

export function useSaveAd(id?: UUID) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: SaveLoadAd) =>
      id ? api.put<LoadAd>(`/load-ads/${id}`, body) : api.post<LoadAd>("/load-ads", body),
    onSuccess: (ad) => {
      qc.setQueryData(keys.ad(ad.id), ad);
      void qc.invalidateQueries({ queryKey: keys.ads });
      void qc.invalidateQueries({ queryKey: keys.usage });
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });
}

export function useArchiveAd() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: UUID) => api.del(`/load-ads/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.ads });
      void qc.invalidateQueries({ queryKey: ["history"] });
    },
  });
}

// ── distribution ────────────────────────────────────────────────────

export function usePreview(adId: UUID) {
  return useMutation({
    mutationFn: (accountId?: UUID) =>
      api.post<PublishPreview>(`/load-ads/${adId}/publish/preview`, undefined, {
        query: { account_id: accountId },
      }),
  });
}

export function usePublish(adId: UUID) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { previewToken: string; groupIds: UUID[]; idempotencyKey: string }) =>
      api.post<PublishResult>(
        `/load-ads/${adId}/publish`,
        { preview_token: p.previewToken, group_ids: p.groupIds },
        { headers: { "Idempotency-Key": p.idempotencyKey } },
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.ads });
      void qc.invalidateQueries({ queryKey: keys.adPosts(adId) });
      void qc.invalidateQueries({ queryKey: ["history"] });
      void qc.invalidateQueries({ queryKey: keys.usage });
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });
}

/** Refreshes while anything is still on its way, and stops once it is not. */
export function useAdPosts(adId: UUID) {
  return useQuery({
    queryKey: keys.adPosts(adId),
    queryFn: () => api.get<PostingJob[]>(`/load-ads/${adId}/posts`),
    refetchInterval: (query) =>
      query.state.data?.some((j) => ["pending", "processing", "rate_limited"].includes(j.status)) ? 10_000 : false,
  });
}

export interface HistoryFilters {
  status?: string;
  mine?: boolean;
  loadAdId?: UUID;
  page: number;
}

export function useHistory(filters: HistoryFilters) {
  return useQuery({
    queryKey: keys.history(filters),
    queryFn: () =>
      api.page<PostingJob & { load_ad_id?: UUID }>("/posting-history", {
        status: filters.status,
        mine: filters.mine || undefined,
        load_ad_id: filters.loadAdId,
        page: filters.page,
        per_page: 25,
      }),
    placeholderData: keepPreviousData,
    refetchInterval: 30_000,
  });
}

export function useCancelJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (jobId: UUID) => api.post(`/posting-jobs/${jobId}/cancel`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["history"] });
      void qc.invalidateQueries({ queryKey: ["ads", "posts"] });
      void qc.invalidateQueries({ queryKey: keys.usage });
    },
  });
}

// ── telegram ────────────────────────────────────────────────────────

export function useAccounts(company = false) {
  return useQuery({
    queryKey: keys.accounts(company),
    queryFn: () => api.get<TelegramAccount[]>("/telegram/accounts", { company: company || undefined }),
  });
}

function useInvalidateTelegram() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ["telegram"] });
    void qc.invalidateQueries({ queryKey: keys.usage });
    void qc.invalidateQueries({ queryKey: ["dashboard"] });
  };
}

export function useStartLogin() {
  return useMutation({
    mutationFn: (body: { phone: string; label?: string; consent_version: string }) =>
      api.post<TelegramLoginStep>("/telegram/accounts/login", body),
  });
}

export function useSubmitCode() {
  const invalidate = useInvalidateTelegram();
  return useMutation({
    mutationFn: (p: { accountId: UUID; code: string }) =>
      api.post<TelegramLoginStep>(`/telegram/accounts/${p.accountId}/code`, { code: p.code }),
    onSuccess: (step) => {
      if (step.step === "connected") invalidate();
    },
  });
}

export function useSubmitPassword() {
  const invalidate = useInvalidateTelegram();
  return useMutation({
    mutationFn: (p: { accountId: UUID; password: string }) =>
      api.post<TelegramLoginStep>(`/telegram/accounts/${p.accountId}/password`, { password: p.password }),
    onSuccess: invalidate,
  });
}

export function useDisconnect() {
  const invalidate = useInvalidateTelegram();
  return useMutation({
    mutationFn: (accountId: UUID) => api.del(`/telegram/accounts/${accountId}`),
    onSuccess: invalidate,
  });
}

export function useDialogs(accountId: UUID | undefined) {
  return useQuery({
    queryKey: keys.dialogs(accountId ?? ""),
    queryFn: () => api.get<TelegramDialog[]>(`/telegram/accounts/${accountId}/dialogs`),
    enabled: Boolean(accountId),
    staleTime: 60_000,
    retry: false,
  });
}

export function useGroups() {
  return useQuery({ queryKey: keys.groups, queryFn: () => api.get<TelegramGroup[]>("/telegram/groups") });
}

export function useConnectGroup() {
  const invalidate = useInvalidateTelegram();
  return useMutation({
    mutationFn: (body: { account_id: UUID; peer_id: string; language: string }) =>
      api.post<TelegramGroup>("/telegram/groups", body),
    onSuccess: invalidate,
  });
}

export function useSaveRules(groupId: UUID) {
  const invalidate = useInvalidateTelegram();
  return useMutation({
    mutationFn: (body: SaveGroupRules) => api.put<TelegramGroup>(`/telegram/groups/${groupId}/rules`, body),
    onSuccess: invalidate,
  });
}

export function useRemoveGroup() {
  const invalidate = useInvalidateTelegram();
  return useMutation({
    mutationFn: (groupId: UUID) => api.del(`/telegram/groups/${groupId}`),
    onSuccess: invalidate,
  });
}

// ── notifications ───────────────────────────────────────────────────

export function useUnreadCount() {
  return useQuery({
    queryKey: keys.unread,
    queryFn: () => api.get<{ notifications: number }>("/notifications/unread-count"),
    refetchInterval: 60_000,
  });
}

export function useNotifications(unreadOnly: boolean, page: number) {
  return useQuery({
    queryKey: keys.notifications(unreadOnly, page),
    queryFn: () =>
      api.page<Notification>("/notifications", { unread_only: unreadOnly || undefined, page, per_page: 20 }),
    placeholderData: keepPreviousData,
  });
}

export function useMarkRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids?: UUID[]) => api.post("/notifications/read", { ids: ids ?? [] }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["notifications"] }),
  });
}
