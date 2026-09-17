import { Archive, ArrowLeft, Pencil } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useErrorText, useToast } from "../../components/feedback";
import { AdStatusBadge, JobStatusBadge } from "../../components/status";
import { Button, Card, ConfirmDialog, ErrorState, LinkButton, Notice, PageHeader, Spinner } from "../../components/ui";
import { useI18n } from "../../i18n";
import { useAuth } from "../../lib/auth";
import { formatDate, formatMoney, formatWeight, formatWhen } from "../../lib/format";
import { useAd, useAdPosts, useArchiveAd, useCancelJob, useCargoTypes, useVehicleTypes } from "../../lib/queries";
import type { LoadAd, PostingJob } from "../../lib/types";
import { routeLabel } from "./adForm";
import { PublishPanel } from "./PublishPanel";
import { fieldText, reasonText } from "../../lib/labels";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5 py-2 sm:flex-row sm:gap-4">
      <dt className="text-sm text-muted sm:w-44 sm:shrink-0">{label}</dt>
      <dd className="text-sm text-ink">{children || "—"}</dd>
    </div>
  );
}

function Details({ ad }: { ad: LoadAd }) {
  const { t } = useI18n();
  const vehicles = useVehicleTypes();
  const cargoTypes = useCargoTypes();
  const vehicle = vehicles.data?.find((v) => v.code === ad.vehicle_type)?.name ?? ad.vehicle_type;
  const cargo = cargoTypes.data?.find((c) => c.id === ad.cargo_type_id)?.name;
  const payment = [
    ad.payment_terms && t(`pay.${ad.payment_terms}` as "pay.on_loading"),
    ad.payment_method && t(`pm.${ad.payment_method}` as "pm.cash"),
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <dl className="divide-y divide-line">
      <Row label={t("field.origin")}>{ad.origin.text || ad.origin.country}</Row>
      <Row label={t("field.destination")}>{ad.destination.text || ad.destination.country}</Row>
      <Row label={t("field.loading_date")}>
        {ad.loading_date ? formatDate(ad.loading_date) : ad.loading_date_text}
      </Row>
      <Row label={t("field.weight_kg")}>{ad.weight_kg ? formatWeight(ad.weight_kg) : null}</Row>
      <Row label={t("field.volume_m3")}>{ad.volume_m3 ? `${ad.volume_m3} m³` : null}</Row>
      <Row label={t("field.vehicle_type")}>{vehicle}</Row>
      <Row label={t("field.cargo_type")}>{[cargo, ad.cargo_text].filter(Boolean).join(" · ")}</Row>
      <Row label={t("field.price")}>
        {ad.price_amount !== null && ad.price_amount !== undefined
          ? `${formatMoney(ad.price_amount, ad.price_currency)}${ad.price_negotiable ? ` · ${t("field.negotiable")}` : ""}`
          : ad.price_negotiable
            ? t("field.negotiable")
            : null}
      </Row>
      <Row label={t("field.payment_terms")}>{payment}</Row>
      <Row label={t("field.contact_phone")}>
        {ad.contact_phone && (
          <a href={`tel:${ad.contact_phone}`} className="text-brand hover:underline">
            {ad.contact_phone}
          </a>
        )}
        {ad.contact_name && ` · ${ad.contact_name}`}
      </Row>
      <Row label={t("field.notes")}>{ad.notes}</Row>
    </dl>
  );
}

function Posts({ adId }: { adId: string }) {
  const { t } = useI18n();
  const { can } = useAuth();
  const toast = useToast();
  const errorText = useErrorText();
  const posts = useAdPosts(adId);
  const cancel = useCancelJob();

  const detail = (job: PostingJob) => {
    if (job.skip_reason) return reasonText(job.skip_reason, t);
    if (job.error_code) return reasonText(job.error_code, t);
    if (job.next_try_at && (job.status === "pending" || job.status === "rate_limited")) {
      return t("hist.nextTry", { time: formatWhen(job.next_try_at) });
    }
    return "";
  };

  return (
    <Card title={t("ad.posts")} padded={false}>
      {posts.isLoading ? (
        <Spinner />
      ) : posts.error ? (
        <ErrorState message={errorText(posts.error)} onRetry={() => void posts.refetch()} />
      ) : !posts.data || posts.data.length === 0 ? (
        <p className="px-5 py-8 text-center text-sm text-muted">{t("ad.postsEmpty")}</p>
      ) : (
        <ul className="divide-y divide-line">
          {posts.data.map((job) => {
            const cancellable = ["pending", "rate_limited"].includes(job.status);
            return (
              <li key={job.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-ink">{job.group_title || job.group_id}</p>
                  <p className="text-xs text-muted">
                    {formatWhen(job.sent_at ?? job.created_at)}
                    {job.attempt > 1 ? ` · #${job.attempt}` : ""}
                    {detail(job) && ` · ${detail(job)}`}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <JobStatusBadge status={job.status} />
                  {cancellable && can("load_ads.publish") && (
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
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}

export function AdDetailPage() {
  const { id = "" } = useParams();
  const { t } = useI18n();
  const { can } = useAuth();
  const navigate = useNavigate();
  const toast = useToast();
  const errorText = useErrorText();
  const ad = useAd(id);
  const archive = useArchiveAd();
  const [confirming, setConfirming] = useState(false);

  if (ad.isLoading) return <Spinner />;
  if (ad.error || !ad.data) {
    return <ErrorState message={errorText(ad.error)} onRetry={() => void ad.refetch()} />;
  }
  const data = ad.data;
  const archived = data.status === "archived";

  return (
    <>
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-3">
            {routeLabel(data)}
            <AdStatusBadge status={data.status} />
          </span>
        }
        subtitle={`${t("ads.col.created")}: ${formatWhen(data.created_at)}`}
        back={
          <Link to="/ads" className="mb-2 inline-flex items-center gap-1 text-sm text-muted hover:text-ink">
            <ArrowLeft className="size-4" aria-hidden />
            {t("ads.title")}
          </Link>
        }
        actions={
          !archived && (
            <>
              {can("load_ads.create") && (
                <LinkButton to={`/ads/${data.id}/edit`} variant="secondary" icon={<Pencil className="size-4" />}>
                  {t("common.edit")}
                </LinkButton>
              )}
              {can("load_ads.delete") && (
                <Button variant="danger" icon={<Archive className="size-4" />} onClick={() => setConfirming(true)}>
                  {t("ad.archive")}
                </Button>
              )}
            </>
          )
        }
      />

      <div className="flex flex-col gap-6">
        {data.missing.length > 0 && !archived && (
          <Notice tone="warn">
            {t("ad.notReady", { fields: data.missing.map((f) => fieldText(f, t)).join(", ") })}
          </Notice>
        )}

        <div className="grid gap-6 lg:grid-cols-5">
          <div className="flex flex-col gap-6 lg:col-span-3">
            {!archived && can("load_ads.publish") && <PublishPanel ad={data} />}
            {can("posting.view") && <Posts adId={data.id} />}
          </div>
          <div className="flex flex-col gap-6 lg:col-span-2">
            <Card title={t("ad.details")}>
              <Details ad={data} />
            </Card>
            {data.raw_text && (
              <Card title={t("ad.rawText")}>
                <p className="text-sm whitespace-pre-wrap text-ink-soft">{data.raw_text}</p>
              </Card>
            )}
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={confirming}
        message={t("ad.archiveConfirm")}
        confirmLabel={t("ad.archive")}
        loading={archive.isPending}
        onClose={() => setConfirming(false)}
        onConfirm={() =>
          archive.mutate(data.id, {
            onSuccess: () => {
              toast.success(t("ad.archived"));
              navigate("/ads");
            },
            onError: (err) => {
              setConfirming(false);
              toast.error(errorText(err));
            },
          })
        }
      />
    </>
  );
}
