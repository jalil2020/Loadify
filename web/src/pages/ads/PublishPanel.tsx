import { CircleCheck, Send, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useErrorText, useToast } from "../../components/feedback";

import { Badge, Button, Card, Checkbox, LinkButton, Notice, Select } from "../../components/ui";
import { useI18n } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { useAccounts, useGroups, usePreview, usePublish } from "../../lib/queries";
import type { LoadAd, PublishPreview, UUID } from "../../lib/types";
import { matchText, reasonText } from "../../lib/labels";

/** Ticks while a preview is open, so its expiry shows without a reload. */
function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const id = window.setInterval(() => setNow(Date.now()), 15_000);
    return () => window.clearInterval(id);
  }, [active]);
  return now;
}

export function PublishPanel({ ad }: { ad: LoadAd }) {
  const { t, lang } = useI18n();
  const toast = useToast();
  const errorText = useErrorText();
  const accounts = useAccounts();
  const groups = useGroups();
  const preview = usePreview(ad.id);
  const publish = usePublish(ad.id);

  const usable = useMemo(
    () => (accounts.data ?? []).filter((a) => a.mine && (a.status === "active" || a.status === "paused")),
    [accounts.data],
  );
  const [accountId, setAccountId] = useState<UUID | "">("");
  const [review, setReview] = useState<PublishPreview | null>(null);
  const [selected, setSelected] = useState<Set<UUID>>(new Set());
  // One key per review: pressing publish twice, or a retried request, cannot
  // queue the advert twice.
  const [idempotencyKey, setIdempotencyKey] = useState("");
  const now = useNow(review !== null);

  const chosenAccount = accountId || usable[0]?.id || "";
  // An empty result means one of two things: no group is connected at all, or
  // the connected ones did not fit this advert.
  const noGroupsYet = groups.isSuccess && groups.data.length === 0;
  const expired = review !== null && new Date(review.expires_at).getTime() <= now;
  const overQuota = review?.posts_left !== null && review !== null && selected.size > (review.posts_left ?? 0);
  const blocked = ad.missing.length > 0 || ad.status === "archived";

  const runReview = () => {
    preview.mutate(chosenAccount || undefined, {
      onSuccess: (res) => {
        setReview(res);
        setSelected(new Set(res.approved.map((g) => g.group_id)));
        setIdempotencyKey(crypto.randomUUID());
      },
      onError: (err) => toast.error(errorText(err)),
    });
  };

  const submit = () => {
    if (!review) return;
    publish.mutate(
      { previewToken: review.preview_token, groupIds: [...selected], idempotencyKey },
      {
        onSuccess: (res) => {
          toast.success(
            res.skipped > 0
              ? t("pub.doneSkipped", { queued: res.queued, skipped: res.skipped })
              : t("pub.done", { queued: res.queued }),
          );
          setReview(null);
        },
        onError: (err) => toast.error(errorText(err)),
      },
    );
  };

  if (accounts.isSuccess && usable.length === 0) {
    return (
      <Card title={t("pub.title")}>
        <div className="flex flex-col items-start gap-3">
          <p className="text-sm text-ink-soft">{t("pub.noAccount")}</p>
          <LinkButton to="/telegram" variant="secondary" icon={<Send className="size-4" />}>
            {t("pub.connect")}
          </LinkButton>
        </div>
      </Card>
    );
  }

  return (
    <Card title={t("pub.title")}>
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-3">
          {usable.length > 1 && (
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="font-medium text-ink-soft">{t("pub.account")}</span>
              <Select
                value={chosenAccount}
                onChange={(e) => {
                  setAccountId(e.target.value);
                  setReview(null);
                }}
                className="min-w-56"
              >
                {usable.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.label ? `${a.label} · ${a.phone}` : a.phone}
                  </option>
                ))}
              </Select>
            </label>
          )}
          <Button
            variant={review ? "secondary" : "primary"}
            onClick={runReview}
            loading={preview.isPending}
            disabled={blocked}
          >
            {preview.isPending ? t("pub.reviewing") : t("pub.review")}
          </Button>
        </div>

        <p className="flex items-center gap-2 text-xs text-muted">
          <ShieldCheck className="size-4 shrink-0" aria-hidden />
          {t("pub.safety")}
        </p>

        {review && (
          <div className="flex flex-col gap-4 border-t border-line pt-4">
            <p className="text-sm text-ink-soft">
              {t("pub.funnel", {
                candidates: review.candidates,
                approved: review.approved.length,
                skipped: review.skipped.length,
              })}
            </p>

            {review.candidates === 0 && review.approved.length === 0 && review.skipped.length === 0 ? (
              // Nothing matched has two quite different causes, and telling
              // someone to check rules they never wrote sends them looking in
              // the wrong place.
              <Notice tone="warn">
                <div className="flex flex-wrap items-center gap-3">
                  <span>{noGroupsYet ? t("pub.noGroups") : t("pub.noCandidates")}</span>
                  {noGroupsYet && (
                    <LinkButton to="/telegram" variant="secondary" size="sm" icon={<Send className="size-4" />}>
                      {t("pub.connectGroup")}
                    </LinkButton>
                  )}
                </div>
              </Notice>
            ) : (
              <div className="grid gap-4 lg:grid-cols-2">
                <div>
                  <div className="mb-2 flex items-center justify-between">
                    <h3 className="text-sm font-semibold text-ink">
                      {t("pub.approved")} ({selected.size}/{review.approved.length})
                    </h3>
                    {review.approved.length > 1 && (
                      <Checkbox
                        checked={selected.size === review.approved.length}
                        onChange={(on) => setSelected(on ? new Set(review.approved.map((g) => g.group_id)) : new Set())}
                        label={t("common.all")}
                      />
                    )}
                  </div>
                  <ul className="flex flex-col gap-2">
                    {review.approved.map((g) => (
                      <li key={g.group_id} className="rounded-lg border border-line p-3">
                        <div className="flex items-start justify-between gap-3">
                          <Checkbox
                            checked={selected.has(g.group_id)}
                            onChange={(on) =>
                              setSelected((prev) => {
                                const next = new Set(prev);
                                if (on) next.add(g.group_id);
                                else next.delete(g.group_id);
                                return next;
                              })
                            }
                            label={<span className="font-medium text-ink">{g.title}</span>}
                          />
                          {g.score !== undefined && <Badge tone="brand">{t("pub.score", { n: g.score })}</Badge>}
                        </div>
                        {g.reasons && g.reasons.length > 0 && (
                          <div className="mt-2 flex flex-wrap gap-1">
                            {g.reasons.map((r) => (
                              <Badge key={r}>{matchText(r, t)}</Badge>
                            ))}
                          </div>
                        )}
                      </li>
                    ))}
                    {review.approved.length === 0 && <li className="text-sm text-muted">—</li>}
                  </ul>
                </div>

                <div>
                  <h3 className="mb-2 text-sm font-semibold text-ink">
                    {t("pub.skipped")} ({review.skipped.length})
                  </h3>
                  <ul className="flex flex-col gap-2">
                    {review.skipped.map((g) => (
                      <li key={g.group_id} className="rounded-lg border border-line bg-sunken/50 p-3">
                        <p className="text-sm font-medium text-ink-soft">{g.title}</p>
                        <p className="text-xs text-muted">
                          {reasonText(g.reason, t)}
                          {g.retry_after_seconds
                            ? ` · ${t("pub.retryIn", { time: formatDuration(g.retry_after_seconds, lang) })}`
                            : ""}
                        </p>
                      </li>
                    ))}
                    {review.skipped.length === 0 && <li className="text-sm text-muted">—</li>}
                  </ul>
                </div>
              </div>
            )}

            <div>
              <h3 className="mb-2 text-sm font-semibold text-ink">{t("pub.message")}</h3>
              <pre className="max-h-64 overflow-auto rounded-lg border border-line bg-sunken p-3 font-sans text-sm whitespace-pre-wrap text-ink">
                {review.message_preview}
              </pre>
            </div>

            {review.posts_left !== null && (
              <Notice tone={overQuota ? "bad" : "info"}>
                {overQuota ? t("pub.overQuota") : t("pub.postsLeft", { n: review.posts_left })}
              </Notice>
            )}
            {expired && <Notice tone="warn">{t("pub.expired")}</Notice>}

            <div className="flex justify-end">
              <Button
                onClick={submit}
                loading={publish.isPending}
                disabled={selected.size === 0 || expired || overQuota}
                icon={<CircleCheck className="size-4" />}
              >
                {t("pub.submit", { n: selected.size })}
              </Button>
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}
