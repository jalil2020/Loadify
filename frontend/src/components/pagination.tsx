"use client";

import { Button } from "@/components/ui/primitives";
import { useLocale } from "@/components/providers";
import { formatNumber } from "@/lib/format";
import type { PageMeta } from "@/lib/api";

export function Pagination({ meta, onPage }: { meta: PageMeta; onPage: (page: number) => void }) {
  const { t, locale } = useLocale();
  if (meta.total_pages <= 1) return null;

  return (
    <nav className="flex items-center justify-between gap-3" aria-label="Pagination">
      <p className="text-sm text-muted-foreground">
        {t("common.page")} {formatNumber(meta.page, locale)} {t("common.of")}{" "}
        {formatNumber(meta.total_pages, locale)}
      </p>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={meta.page <= 1}
          onClick={() => onPage(meta.page - 1)}
        >
          {t("common.prev")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={meta.page >= meta.total_pages}
          onClick={() => onPage(meta.page + 1)}
        >
          {t("common.next")}
        </Button>
      </div>
    </nav>
  );
}
