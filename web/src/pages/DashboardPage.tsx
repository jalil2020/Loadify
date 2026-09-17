import { clsx } from "clsx";
import { Plus } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { useErrorText } from "../components/feedback";
import { QuotaList } from "../components/quota";
import { AccountStatusBadge, JobStatusBadge } from "../components/status";
import { Badge, Card, ErrorState, LinkButton, PageHeader, Skeleton } from "../components/ui";
import { useI18n } from "../i18n";
import { useAuth } from "../lib/auth";
import { formatDate, formatNumber, formatPercent, formatUSD, formatWhen } from "../lib/format";
import { useDashboard } from "../lib/queries";
import type { AccountStatus, Dashboard } from "../lib/types";
import { reasonText } from "../lib/labels";

function Stat({ label, value, hint, children }: { label: string; value: ReactNode; hint?: ReactNode; children?: ReactNode }) {
  return (
    <div className="rounded-xl border border-line bg-surface p-5">
      <p className="text-sm text-muted">{label}</p>
      <p className="mt-1 text-3xl font-semibold tracking-tight text-ink tabular">{value}</p>
      {hint && <p className="mt-1 text-xs text-muted">{hint}</p>}
      {children && <div className="mt-3">{children}</div>}
    </div>
  );
}

function PostBreakdown({ posts }: { posts: Dashboard["posts"]["today"] }) {
  const { t } = useI18n();
  const rows = [
    ["sent", posts.sent, "bg-ok"],
    ["pending", posts.queued, "bg-info"],
    ["failed", posts.failed, "bg-bad"],
    ["skipped", posts.skipped, "bg-line-strong"],
  ] as const;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex h-2 overflow-hidden rounded-full bg-sunken" aria-hidden>
        {posts.total > 0 &&
          rows.map(([key, n, color]) =>
            n > 0 ? <div key={key} className={color} style={{ width: `${(n / posts.total) * 100}%` }} /> : null,
          )}
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        {rows.map(([key, n, color]) => (
          <div key={key} className="flex items-center justify-between gap-2">
            <dt className="flex items-center gap-1.5 text-muted">
              <span className={clsx("size-2 rounded-full", color)} aria-hidden />
              {t(`status.${key}`)}
            </dt>
            <dd className="font-medium text-ink tabular">{n}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

export function DashboardPage() {
  const { t } = useI18n();
  const { isManager, can } = useAuth();
  const errorText = useErrorText();
  const [scope, setScope] = useState<"company" | "mine" | undefined>(undefined);
  const { data, isLoading, error, refetch } = useDashboard(scope);

  const scopeSwitch = isManager && data && (
    <div role="group" className="flex rounded-lg border border-line bg-surface p-0.5 text-sm">
      {(["company", "mine"] as const).map((s) => (
        <button
          key={s}
          type="button"
          aria-pressed={data.scope === s}
          onClick={() => setScope(s)}
          className={clsx(
            "rounded-md px-3 py-1.5",
            data.scope === s ? "bg-brand-soft font-medium text-brand-strong" : "text-ink-soft hover:bg-sunken",
          )}
        >
          {t(`dash.scope.${s}`)}
        </button>
      ))}
    </div>
  );

  return (
    <>
      <PageHeader
        title={t("dash.title")}
        subtitle={data ? t("dash.today", { day: formatDate(data.day) }) : undefined}
        actions={
          <>
            {scopeSwitch}
            {can("load_ads.create") && (
              <LinkButton to="/ads/new" icon={<Plus className="size-4" />}>{t("dash.newAd")}</LinkButton>
            )}
          </>
        }
      />

      {error ? (
        <Card>
          <ErrorState message={errorText(error)} onRetry={() => void refetch()} />
        </Card>
      ) : isLoading || !data ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-32" />
          ))}
        </div>
      ) : (
        <div className="flex flex-col gap-6">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Stat
              label={t("dash.loads")}
              value={formatNumber(data.loads.today)}
              hint={t("dash.loadsHint", { month: formatNumber(data.loads.month) })}
            />
            <Stat label={t("dash.posts")} value={formatNumber(data.posts.today.total)}>
              <PostBreakdown posts={data.posts.today} />
            </Stat>
            <Stat
              label={t("dash.successRate")}
              value={formatPercent(data.posts.success_rate)}
              hint={
                <>
                  {t("dash.successHint")}
                  <br />
                  {t("dash.lastSent")}: {data.posts.last_sent_at ? formatWhen(data.posts.last_sent_at) : t("dash.never")}
                </>
              }
            />
            <Stat label={t("dash.accounts")} value={formatNumber(data.accounts.total)}>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(data.accounts.by_status).map(([status, n]) => (
                  <span key={status} className="inline-flex items-center gap-1 text-xs">
                    <AccountStatusBadge status={status as AccountStatus} />
                    <span className="text-muted tabular">×{n}</span>
                  </span>
                ))}
              </div>
            </Stat>
            <Stat
              label={t("dash.groups")}
              value={formatNumber(data.groups.connected)}
              hint={t("dash.groupsHint", { postable: formatNumber(data.groups.postable) })}
            />
            {data.ai.enabled ? (
              <Stat
                label={t("dash.ai")}
                value={formatNumber(data.ai.requests_today)}
                hint={t("dash.aiHint", {
                  month: formatNumber(data.ai.requests_month),
                  cache: formatNumber(data.ai.cache_hits_month),
                })}
              >
                <p className="text-xs text-muted">
                  {t("dash.aiCost")}: <span className="font-medium text-ink">{formatUSD(data.ai.cost_month_usd)}</span>
                </p>
              </Stat>
            ) : (
              <Stat label={t("dash.ai")} value="—" hint={t("dash.aiOff")} />
            )}
          </div>

          <div className="grid gap-6 lg:grid-cols-3">
            <Card title={t("dash.problems")} className="lg:col-span-2" padded={false}>
              {data.recent_problems.length === 0 ? (
                <p className="px-5 py-8 text-center text-sm text-muted">{t("dash.problemsEmpty")}</p>
              ) : (
                <ul className="divide-y divide-line">
                  {data.recent_problems.map((p) => (
                    <li key={p.job_id} className="flex flex-wrap items-center justify-between gap-2 px-5 py-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-ink">{p.group_title}</p>
                        <p className="text-xs text-muted">{reasonText(p.reason, t)}</p>
                      </div>
                      <div className="flex items-center gap-3">
                        <JobStatusBadge status={p.status} />
                        <span className="text-xs text-muted tabular">{formatWhen(p.at)}</span>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </Card>

            <Card title={t("dash.guard")}>
              {Object.keys(data.guard_today).length === 0 ? (
                <p className="text-sm text-muted">{t("dash.guardEmpty")}</p>
              ) : (
                <ul className="flex flex-col gap-2">
                  {Object.entries(data.guard_today)
                    .sort((a, b) => b[1] - a[1])
                    .map(([kind, n]) => (
                      <li key={kind} className="flex items-center justify-between gap-3 text-sm">
                        <span className="text-ink-soft">{reasonText(kind, t)}</span>
                        <Badge>{n}</Badge>
                      </li>
                    ))}
                </ul>
              )}
            </Card>
          </div>

          <Card
            title={t("dash.quota")}
            action={
              <Link to="/usage" className="text-sm text-brand hover:underline">
                {t("nav.usage")}
              </Link>
            }
          >
            <QuotaList items={data.quota} />
          </Card>
        </div>
      )}
    </>
  );
}
