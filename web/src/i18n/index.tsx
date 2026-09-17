import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { setApiLocale } from "../lib/api";
import { interpolate, type Lang } from "./core";
import { ru } from "./ru";
import { uz, type MessageKey } from "./uz";

export { LANGS, type Lang } from "./core";

const dictionaries: Record<Lang, Record<MessageKey, string>> = { uz, ru };
const STORAGE_KEY = "loadify.lang";

export type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;

interface I18n {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: Translate;
}

const I18nContext = createContext<I18n | null>(null);

function initialLang(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "uz" || saved === "ru") return saved;
  } catch {
    // no storage: fall through to the default
  }
  return "uz";
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(() => {
    const value = initialLang();
    setApiLocale(value);
    return value;
  });

  const setLang = useCallback((value: Lang) => {
    setApiLocale(value);
    document.documentElement.lang = value;
    try {
      localStorage.setItem(STORAGE_KEY, value);
    } catch {
      // the choice then lasts for this page only
    }
    setLangState(value);
  }, []);

  const t = useCallback<Translate>(
    (key, vars) => interpolate(dictionaries[lang][key] ?? uz[key] ?? key, vars),
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18n {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n outside I18nProvider");
  return ctx;
}

export type { MessageKey };
