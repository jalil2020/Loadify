import { useI18n, type MessageKey } from "../i18n";
import { formatDate, formatNumber } from "../lib/format";
import type { QuotaItem } from "../lib/types";
import { Meter } from "./ui";

export function QuotaList({ items, compact }: { items: QuotaItem[]; compact?: boolean }) {
  const { t } = useI18n();
  return (
    <ul className={compact ? "flex flex-col gap-4" : "grid gap-5 sm:grid-cols-2"}>
      {items.map((item) => {
        const limited = item.limit !== null;
        const full = limited && item.remaining === 0;
        return (
          <li key={item.metric} className="flex flex-col gap-2">
            <div className="flex items-baseline justify-between gap-3 text-sm">
              <span className="font-medium text-ink">{t(`metric.${item.metric}` as MessageKey)}</span>
              <span className="text-muted tabular">
                {formatNumber(item.used)} / {limited ? formatNumber(item.limit) : t("common.unlimited")}
              </span>
            </div>
            {limited ? <Meter value={item.used} max={item.limit ?? 0} /> : <Meter value={0} max={1} tone="ok" />}
            {!compact && (
              <p className={full ? "text-xs text-bad" : "text-xs text-muted"}>
                {full
                  ? t("usage.full")
                  : limited
                    ? t("usage.left", { n: formatNumber(item.remaining) })
                    : t("common.unlimited")}
                {item.resets_at && ` · ${t("usage.resets", { date: formatDate(item.resets_at) })}`}
              </p>
            )}
          </li>
        );
      })}
    </ul>
  );
}
