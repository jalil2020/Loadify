import { Check, CreditCard } from "lucide-react";
import { useState } from "react";
import { useErrorText, useToast } from "../components/feedback";
import { QuotaList } from "../components/quota";
import {
  Badge,
  Button,
  Card,
  Checkbox,
  ConfirmDialog,
  ErrorState,
  Field,
  Modal,
  Notice,
  PageHeader,
  Select,
  Skeleton,
  Spinner,
} from "../components/ui";
import { useI18n, type MessageKey, type Translate } from "../i18n";
import { useAuth } from "../lib/auth";
import { formatDate, formatMoney } from "../lib/format";
import {
  useCancelSubscription,
  usePaymentMethods,
  usePlans,
  useSubscribe,
  useSubscription,
  useUsage,
} from "../lib/queries";
import type { Plan } from "../lib/types";

/** The allowances this product runs on. A plan's other keys belong to the
 * marketplace and would only be noise here. */
const SHOWN_LIMITS = ["tg_accounts", "tg_groups", "loads", "posts", "ai_requests"] as const;

/** A limit that is -1, or absent, is no ceiling at all. */
function limitText(plan: Plan, metric: string, t: Translate): string {
  const value = plan.limits[metric];
  return value === undefined || value === -1 ? t("common.unlimited") : String(value);
}

function priceText(plan: Plan, t: Translate): string {
  if (plan.price_amount === 0) return t("plan.free");
  const period = plan.period === "year" ? t("plan.perYear") : t("plan.perMonth");
  return `${formatMoney(plan.price_amount, plan.currency)} ${period}`;
}

function PlanCard({ plan, onChoose }: { plan: Plan; onChoose: (plan: Plan) => void }) {
  const { t } = useI18n();
  const { can } = useAuth();
  return (
    <Card label={plan.name}>
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h3 className="text-base font-semibold text-ink">{plan.name}</h3>
            <p className="text-sm text-muted">{priceText(plan, t)}</p>
          </div>
          {plan.is_current && <Badge tone="ok">{t("plan.current")}</Badge>}
        </div>

        <dl className="flex flex-col gap-1.5 text-sm">
          {SHOWN_LIMITS.map((metric) => (
            <div key={metric} className="flex items-baseline justify-between gap-3">
              <dt className="text-muted">{t(`metric.${metric}` as MessageKey)}</dt>
              <dd className="font-medium text-ink tabular">{limitText(plan, metric, t)}</dd>
            </div>
          ))}
        </dl>

        {plan.trial_days > 0 && !plan.is_current && (
          <p className="flex items-center gap-1.5 text-xs text-ok">
            <Check className="size-3.5" aria-hidden />
            {t("plan.trial", { n: plan.trial_days })}
          </p>
        )}

        {can("payments.manage") && (
          <Button
            variant={plan.is_current ? "secondary" : "primary"}
            disabled={plan.is_current}
            onClick={() => onChoose(plan)}
            icon={plan.price_amount > 0 ? <CreditCard className="size-4" /> : undefined}
          >
            {plan.is_current ? t("plan.current") : plan.price_amount === 0 ? t("plan.switch") : t("plan.buy")}
          </Button>
        )}
      </div>
    </Card>
  );
}

/** How to pay, and whether to keep paying. The payment itself happens on the
 * provider's own page; nothing card-shaped is ever typed here. */
function CheckoutDialog({ plan, onClose }: { plan: Plan; onClose: () => void }) {
  const { t } = useI18n();
  const toast = useToast();
  const errorText = useErrorText();
  const methods = usePaymentMethods();
  const subscribe = useSubscribe();

  const free = plan.price_amount === 0;
  const providers = methods.data?.providers ?? [];
  const [provider, setProvider] = useState("");
  const [autoRenew, setAutoRenew] = useState(!free);
  const chosen = provider || providers[0] || "wallet";

  const submit = () => {
    subscribe.mutate(
      {
        plan_id: plan.id,
        provider: free ? "wallet" : chosen,
        auto_renew: autoRenew,
        return_url: `${window.location.origin}/usage`,
      },
      {
        onSuccess: (checkout) => {
          if (checkout.checkout_url) {
            // The provider's page finishes the payment and sends the person
            // back; the plan changes when its webhook arrives, not here.
            window.location.href = checkout.checkout_url;
            return;
          }
          toast.success(checkout.paid ? t("plan.bought") : t("plan.pending"));
          onClose();
        },
        onError: (err) => toast.error(errorText(err)),
      },
    );
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={t("plan.checkoutTitle", { plan: plan.name })}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} loading={subscribe.isPending} disabled={!free && providers.length === 0}>
            {free ? t("plan.switch") : t("plan.toPayment")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <p className="text-sm text-ink-soft">{priceText(plan, t)}</p>

        {!free &&
          (methods.isLoading ? (
            <Skeleton className="h-10" />
          ) : providers.length === 0 ? (
            <Notice tone="warn">{t("plan.noProviders")}</Notice>
          ) : (
            <Field label={t("plan.method")}>
              {(id) => (
                <Select id={id} value={chosen} onChange={(e) => setProvider(e.target.value)}>
                  {providers.map((name) => (
                    <option key={name} value={name}>
                      {t(`provider.${name}` as MessageKey)}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          ))}

        <Checkbox checked={autoRenew} onChange={setAutoRenew} label={t("plan.autoRenew")} />
        {!free && <p className="text-xs text-muted">{t("plan.checkoutHint")}</p>}
      </div>
    </Modal>
  );
}

export function PlanPage() {
  const { t } = useI18n();
  const { can } = useAuth();
  const toast = useToast();
  const errorText = useErrorText();

  const usage = useUsage();
  const plans = usePlans();
  const subscription = useSubscription();
  const cancel = useCancelSubscription();

  const [choosing, setChoosing] = useState<Plan | null>(null);
  const [cancelling, setCancelling] = useState(false);

  // The period ends at the start of next month; the last day shown is the day
  // before that.
  const lastDay = (iso: string) => new Date(new Date(iso).getTime() - 1).toISOString();

  const current = subscription.data;
  // No paid subscription means the free tier, which is the default plan.
  const currentPlan = current?.plan ?? plans.data?.find((p) => p.is_default) ?? null;

  return (
    <>
      <PageHeader title={t("usage.title")} subtitle={t("usage.subtitle")} />

      <div className="flex flex-col gap-6">
        <Card title={t("plan.currentTitle")} label={t("plan.currentTitle")}>
          {subscription.isLoading || plans.isLoading ? (
            <Spinner />
          ) : (
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="flex flex-col gap-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-lg font-semibold text-ink">{currentPlan?.name ?? "—"}</span>
                  {current && (
                    <Badge tone={current.status === "active" || current.status === "trialing" ? "ok" : "warn"}>
                      {t(`plan.status.${current.status}` as MessageKey)}
                    </Badge>
                  )}
                </div>
                <p className="text-sm text-muted">
                  {current?.expires_at
                    ? t(current.auto_renew ? "plan.renews" : "plan.until", { date: formatDate(current.expires_at) })
                    : t("plan.freeForever")}
                </p>
              </div>
              {current?.auto_renew && can("payments.manage") && (
                <Button variant="secondary" onClick={() => setCancelling(true)}>
                  {t("plan.stopRenewal")}
                </Button>
              )}
            </div>
          )}
        </Card>

        <Card
          title={
            usage.data
              ? t("usage.period", {
                  from: formatDate(usage.data.period_start),
                  to: formatDate(lastDay(usage.data.period_end)),
                })
              : undefined
          }
        >
          {usage.isLoading ? (
            <Spinner />
          ) : usage.error || !usage.data ? (
            <ErrorState message={errorText(usage.error)} onRetry={() => void usage.refetch()} />
          ) : (
            <QuotaList items={usage.data.items} />
          )}
        </Card>

        <section className="flex flex-col gap-4">
          <h2 className="text-sm font-semibold tracking-wide text-muted uppercase">{t("plan.allTitle")}</h2>
          {plans.error ? (
            <ErrorState message={errorText(plans.error)} onRetry={() => void plans.refetch()} />
          ) : plans.isLoading ? (
            <div className="grid gap-4 lg:grid-cols-3">
              {Array.from({ length: 3 }, (_, i) => (
                <Skeleton key={i} className="h-64" />
              ))}
            </div>
          ) : (
            <>
              <div className="grid gap-4 lg:grid-cols-3">
                {plans.data?.map((plan) => <PlanCard key={plan.id} plan={plan} onChoose={setChoosing} />)}
              </div>
              {!can("payments.manage") && <p className="text-xs text-muted">{t("plan.noPermission")}</p>}
            </>
          )}
        </section>
      </div>

      {choosing && <CheckoutDialog plan={choosing} onClose={() => setChoosing(null)} />}

      <ConfirmDialog
        open={cancelling}
        message={t("plan.stopRenewalConfirm")}
        confirmLabel={t("plan.stopRenewal")}
        loading={cancel.isPending}
        onClose={() => setCancelling(false)}
        onConfirm={() =>
          cancel.mutate(undefined, {
            onSuccess: () => {
              toast.success(t("plan.renewalStopped"));
              setCancelling(false);
            },
            onError: (err) => {
              setCancelling(false);
              toast.error(errorText(err));
            },
          })
        }
      />
    </>
  );
}
