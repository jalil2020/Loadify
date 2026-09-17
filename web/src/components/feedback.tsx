import { clsx } from "clsx";
import { CircleAlert, CircleCheck, X } from "lucide-react";
import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { useI18n } from "../i18n";
import { describeError } from "../lib/labels";

// ── errors ──────────────────────────────────────────────────────────

export function useErrorText() {
  const { t } = useI18n();
  return useCallback((err: unknown) => describeError(err, t), [t]);
}

// ── toasts ──────────────────────────────────────────────────────────

interface Toast {
  id: number;
  tone: "ok" | "bad";
  text: string;
}

interface Toasts {
  success: (text: string) => void;
  error: (text: string) => void;
}

const ToastContext = createContext<Toasts | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const next = useRef(1);

  const dismiss = useCallback((id: number) => setItems((all) => all.filter((t) => t.id !== id)), []);

  const push = useCallback(
    (tone: Toast["tone"], text: string) => {
      const id = next.current++;
      setItems((all) => [...all.slice(-3), { id, tone, text }]);
      window.setTimeout(() => dismiss(id), tone === "bad" ? 8000 : 4000);
    },
    [dismiss],
  );

  const value = useMemo<Toasts>(
    () => ({ success: (text) => push("ok", text), error: (text) => push("bad", text) }),
    [push],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div aria-live="polite" className="fixed right-4 bottom-4 z-50 flex w-[min(24rem,calc(100%-2rem))] flex-col gap-2">
        {items.map((toast) => (
          <div
            key={toast.id}
            role={toast.tone === "bad" ? "alert" : "status"}
            className={clsx(
              "flex items-start gap-2 rounded-lg border bg-surface px-4 py-3 text-sm shadow-lg",
              toast.tone === "bad" ? "border-bad/40" : "border-ok/40",
            )}
          >
            {toast.tone === "bad" ? (
              <CircleAlert className="mt-0.5 size-4 shrink-0 text-bad" aria-hidden />
            ) : (
              <CircleCheck className="mt-0.5 size-4 shrink-0 text-ok" aria-hidden />
            )}
            <p className="flex-1 text-ink">{toast.text}</p>
            <button type="button" onClick={() => dismiss(toast.id)} className="text-muted hover:text-ink">
              <X className="size-4" aria-hidden />
              <span className="sr-only">×</span>
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): Toasts {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast outside ToastProvider");
  return ctx;
}
