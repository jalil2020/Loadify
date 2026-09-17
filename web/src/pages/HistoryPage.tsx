import { useSearchParams } from "react-router";
import { useErrorText, useToast } from "../components/feedback";
import { JobStatusBadge } from "../components/status";
import { Button, Card, Checkbox, EmptyState, ErrorState, PageHeader, Pagination, Select, Skeleton } from "../components/ui";
import { useI18n, type MessageKey } from "../i18n";
import { useAuth } from "../lib/auth";
import { formatDateTime, formatWhen } from "../lib/format";
import { useCancelJob, useHistory } from "../lib/queries";
import type { JobStatus, PostingJob } from "../lib/types";
import { reasonText } from "../lib/labels";

const STATUSES: JobStatus[] = ["pending", "processing", "sent", "failed", "skipped", "rate_limited", "cancelled"];

export function HistoryPage() {
  const { t } = useI18n();
  const { can } = useAuth();
  const toast = useToast();
  const errorText = useErrorText();
  const [params, setParams] = useSearchParams();
  const status = params.get("status") ?? "";
  const mine = params.get("mine") === "1";
  const page = Number(params.get("page") ?? "1") || 1;
  const history = useHistory({ status: status || undefined, mine, page });
  const cancel = useCancelJob();

  const update = (next: Record<string, string | null>) => {
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(next)) {
      if (!v) p.delete(k);
      else p.set(k, v);
    }
    setParams(p, { replace: true });
  };

  const detail = (job: PostingJob) => {
    if (job.skip_reason) return reasonText(job.skip_reason, t);
    if (job.error_code) return reasonText(job.error_code, t);
    if (job.next_try_at && (job.status === "pending" || job.status === "rate_limited")) {
      return t("hist.nextTry", { time: formatWhen(job.next_try_at) });
    }
    return "";
  };

  return (
    <>
      <PageHeader title={t("hist.title")} />
      <Card padded={false}>
        <div className="flex flex-col gap-3 border-b border-line p-4 sm:flex-row sm:items-center">
          <Select
            aria-label={t("hist.col.status")}
            value={status}
            onChange={(e) => update({ status: e.target.value, page: null })}
            className="sm:w-56"
          >
            <option value="">{t("common.all")}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`status.${s}` as MessageKey)}
              </option>
            ))}
          </Select>
          <Checkbox checked={mine} onChange={(v) => update({ mine: v ? "1" : null, page: null })} label={t("hist.mine")} />
        </div>

        {history.error ? (
          <ErrorState message={errorText(history.error)} onRetry={() => void history.refetch()} />
        ) : history.isLoading ? (
          <div className="flex flex-col gap-2 p-4">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-10" />
            ))}
          </div>
        ) : !history.data || history.data.items.length === 0 ? (
          <EmptyState title={t("hist.empty")} />
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[680px] text-sm">
                <thead className="bg-sunken text-left text-xs font-medium tracking-wide text-muted uppercase">
                  <tr>
                    <th className="px-4 py-2.5">{t("hist.col.group")}</th>
                    <th className="px-4 py-2.5">{t("hist.col.status")}</th>
                    <th className="px-4 py-2.5">{t("hist.col.detail")}</th>
                    <th className="px-4 py-2.5 text-right">{t("hist.col.attempt")}</th>
                    <th className="px-4 py-2.5 text-right">{t("hist.col.when")}</th>
                    <th className="px-4 py-2.5" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {history.data.items.map((job) => (
                    <tr key={job.id}>
                      <td className="px-4 py-3 font-medium text-ink">{job.group_title || "—"}</td>
                      <td className="px-4 py-3">
                        <JobStatusBadge status={job.status} />
                      </td>
                      <td className="px-4 py-3 text-xs text-muted">{detail(job)}</td>
                      <td className="px-4 py-3 text-right text-ink-soft tabular">{job.attempt}</td>
                      <td className="px-4 py-3 text-right text-muted tabular" title={formatDateTime(job.created_at)}>
                        {formatWhen(job.sent_at ?? job.created_at)}
                      </td>
                      <td className="px-4 py-3 text-right">
                        {(job.status === "pending" || job.status === "rate_limited") && can("load_ads.publish") && (
                          <Button
                            variant="ghost"
                            size="sm"
                            loading={cancel.isPending && cancel.variables === job.id}
                            onClick={() =>
                              cancel.mutate(job.id, {
                                onSuccess: () => toast.success(t("ad.jobCancelled")),
                                onError: (err) => toast.error(errorText(err)),
                              })
                            }
                          >
                            {t("ad.cancelJob")}
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination
              page={page}
              pages={history.data.meta.total_pages}
              onChange={(p) => update({ page: p > 1 ? String(p) : null })}
            />
          </>
        )}
      </Card>
    </>
  );
}
