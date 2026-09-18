import { Plus, Search } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useErrorText } from "../../components/feedback";
import { AdStatusBadge } from "../../components/status";
import { Card, Checkbox, EmptyState, ErrorState, Input, LinkButton, PageHeader, Pagination, Select, Skeleton } from "../../components/ui";
import { useI18n, type MessageKey } from "../../i18n";
import { useAuth } from "../../lib/auth";
import { formatDate } from "../../lib/format";
import { useAds } from "../../lib/queries";
import type { AdStatus } from "../../lib/types";
import { adTitle } from "./adText";

const STATUSES: AdStatus[] = ["draft", "ready", "publishing", "published", "archived"];

function useDebounced<T>(value: T, delay = 350): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = window.setTimeout(() => setV(value), delay);
    return () => window.clearTimeout(id);
  }, [value, delay]);
  return v;
}

export function AdsPage() {
  const { t } = useI18n();
  const { can } = useAuth();
  const errorText = useErrorText();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const status = params.get("status") ?? "";
  const mine = params.get("mine") === "1";
  const page = Number(params.get("page") ?? "1") || 1;
  const [search, setSearch] = useState(params.get("q") ?? "");
  const debounced = useDebounced(search);

  const update = (next: Record<string, string | null>) => {
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(next)) {
      if (v === null || v === "") p.delete(k);
      else p.set(k, v);
    }
    setParams(p, { replace: true });
  };

  useEffect(() => {
    if ((params.get("q") ?? "") !== debounced) update({ q: debounced, page: null });
    // Only the settled search text should touch the URL.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);

  const ads = useAds({ status: status || undefined, search: debounced || undefined, mine, page });

  return (
    <>
      <PageHeader
        title={t("ads.title")}
        actions={
          can("load_ads.create") && (
            <LinkButton to="/ads/new" icon={<Plus className="size-4" />}>{t("ads.new")}</LinkButton>
          )
        }
      />

      <Card padded={false}>
        <div className="flex flex-col gap-3 border-b border-line p-4 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" aria-hidden />
            <Input
              aria-label={t("common.search")}
              placeholder={t("ads.searchPlaceholder")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9"
            />
          </div>
          <Select
            aria-label={t("ads.col.status")}
            value={status}
            onChange={(e) => update({ status: e.target.value, page: null })}
            className="sm:w-48"
          >
            <option value="">{t("common.all")}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`adStatus.${s}` as MessageKey)}
              </option>
            ))}
          </Select>
          <Checkbox checked={mine} onChange={(v) => update({ mine: v ? "1" : null, page: null })} label={t("ads.mine")} />
        </div>

        {ads.error ? (
          <ErrorState message={errorText(ads.error)} onRetry={() => void ads.refetch()} />
        ) : ads.isLoading ? (
          <div className="flex flex-col gap-2 p-4">
            {Array.from({ length: 5 }, (_, i) => (
              <Skeleton key={i} className="h-12" />
            ))}
          </div>
        ) : !ads.data || ads.data.items.length === 0 ? (
          <EmptyState
            title={t("ads.empty")}
            hint={t("ads.emptyHint")}
            action={
              can("load_ads.create") && (
                <LinkButton to="/ads/new" icon={<Plus className="size-4" />}>{t("ads.new")}</LinkButton>
              )
            }
          />
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[480px] text-sm">
                <thead className="bg-sunken text-left text-xs font-medium tracking-wide text-muted uppercase">
                  <tr>
                    <th className="px-4 py-2.5">{t("ads.col.text")}</th>
                    <th className="px-4 py-2.5">{t("ads.col.status")}</th>
                    <th className="px-4 py-2.5 text-right">{t("ads.col.created")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {ads.data.items.map((ad) => (
                    <tr
                      key={ad.id}
                      onClick={() => navigate(`/ads/${ad.id}`)}
                      className="cursor-pointer hover:bg-sunken/60"
                    >
                      <td className="max-w-0 px-4 py-3">
                        <Link to={`/ads/${ad.id}`} className="block truncate font-medium text-ink hover:text-brand">
                          {adTitle(ad)}
                        </Link>
                      </td>
                      <td className="w-px px-4 py-3 whitespace-nowrap">
                        <AdStatusBadge status={ad.status} />
                      </td>
                      <td className="w-px px-4 py-3 text-right whitespace-nowrap text-muted tabular">
                        {formatDate(ad.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination
              page={page}
              pages={ads.data.meta.total_pages}
              onChange={(p) => update({ page: p > 1 ? String(p) : null })}
            />
          </>
        )}
      </Card>
    </>
  );
}
