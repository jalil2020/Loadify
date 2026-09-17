import { Send } from "lucide-react";
import type { ReactNode } from "react";
import { LANGS, useI18n } from "../../i18n";

export function AuthLayout({ title, subtitle, children }: { title: string; subtitle?: string; children: ReactNode }) {
  const { t, lang, setLang } = useI18n();
  return (
    <div className="flex min-h-full flex-col items-center justify-center px-4 py-10">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center justify-between">
          <div className="flex items-center gap-2 text-lg font-semibold">
            <span className="grid size-9 place-items-center rounded-lg bg-brand text-on-brand">
              <Send className="size-4" aria-hidden />
            </span>
            {t("app.name")}
          </div>
          <div className="flex gap-1 text-xs">
            {LANGS.map((l) => (
              <button
                key={l}
                type="button"
                onClick={() => setLang(l)}
                aria-pressed={lang === l}
                className={
                  lang === l ? "rounded px-2 py-1 font-semibold text-ink" : "rounded px-2 py-1 text-muted hover:text-ink"
                }
              >
                {t(`lang.${l}`)}
              </button>
            ))}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-6 shadow-sm">
          <h1 className="text-xl font-semibold">{title}</h1>
          {subtitle && <p className="mt-1 text-sm text-muted">{subtitle}</p>}
          <div className="mt-6">{children}</div>
        </div>
        <p className="mt-6 text-center text-xs text-muted">{t("app.tagline")}</p>
      </div>
    </div>
  );
}
