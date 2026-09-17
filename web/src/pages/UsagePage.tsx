import { useErrorText } from "../components/feedback";
import { QuotaList } from "../components/quota";
import { Card, ErrorState, PageHeader, Spinner } from "../components/ui";
import { useI18n } from "../i18n";
import { formatDate } from "../lib/format";
import { useUsage } from "../lib/queries";

export function UsagePage() {
  const { t } = useI18n();
  const errorText = useErrorText();
  const usage = useUsage();

  // The period ends at the start of next month; the last day shown is the day
  // before that.
  const lastDay = (iso: string) => new Date(new Date(iso).getTime() - 1).toISOString();

  return (
    <>
      <PageHeader title={t("usage.title")} subtitle={t("usage.subtitle")} />
      <Card
        title={
          usage.data
            ? t("usage.period", { from: formatDate(usage.data.period_start), to: formatDate(lastDay(usage.data.period_end)) })
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
    </>
  );
}
