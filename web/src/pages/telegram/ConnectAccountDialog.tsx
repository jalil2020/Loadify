import { CircleCheck } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useErrorText, useToast } from "../../components/feedback";
import { Button, Checkbox, Field, Input, Modal, Notice } from "../../components/ui";
import { useI18n } from "../../i18n";
import { E164, TELEGRAM_CONSENT_VERSION } from "../../lib/labels";
import { useStartLogin, useSubmitCode, useSubmitPassword } from "../../lib/queries";
import type { UUID } from "../../lib/types";


type Step = "phone" | "code" | "password" | "done";

export function ConnectAccountDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useI18n();
  const toast = useToast();
  const errorText = useErrorText();
  const start = useStartLogin();
  const submitCode = useSubmitCode();
  const submitPassword = useSubmitPassword();

  const [step, setStep] = useState<Step>("phone");
  const [phone, setPhone] = useState("+998");
  const [label, setLabel] = useState("");
  const [consent, setConsent] = useState(false);
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [accountId, setAccountId] = useState<UUID>("");
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    setStep("phone");
    setCode("");
    setPassword("");
    setConsent(false);
    setError(null);
    onClose();
  };

  const after = (next: "code" | "password" | "connected") => {
    setError(null);
    if (next === "connected") {
      setStep("done");
      toast.success(t("tgc.connected"));
    } else {
      setStep(next);
    }
  };

  const onPhone = (e: FormEvent) => {
    e.preventDefault();
    const normalized = phone.replace(/[\s()-]/g, "");
    start.mutate(
      { phone: normalized, label: label.trim() || undefined, consent_version: TELEGRAM_CONSENT_VERSION },
      {
        onSuccess: (res) => {
          setAccountId(res.account_id);
          after(res.step);
        },
        onError: (err) => setError(errorText(err)),
      },
    );
  };

  const onCode = (e: FormEvent) => {
    e.preventDefault();
    submitCode.mutate(
      { accountId, code: code.trim() },
      { onSuccess: (res) => after(res.step), onError: (err) => setError(errorText(err)) },
    );
  };

  const onPassword = (e: FormEvent) => {
    e.preventDefault();
    submitPassword.mutate(
      { accountId, password },
      {
        onSuccess: (res) => {
          setPassword("");
          after(res.step);
        },
        onError: (err) => setError(errorText(err)),
      },
    );
  };

  return (
    <Modal open={open} onClose={close} title={t("tgc.title")}>
      <div className="flex flex-col gap-4">
        {error && <Notice tone="bad">{error}</Notice>}

        {step === "phone" && (
          <form onSubmit={onPhone} className="flex flex-col gap-4">
            <Field label={t("tgc.phone")}>
              {(id) => (
                <Input
                  id={id}
                  type="tel"
                  inputMode="tel"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  autoFocus
                />
              )}
            </Field>
            <Field label={t("tgc.label")} optional>
              {(id) => <Input id={id} maxLength={64} value={label} onChange={(e) => setLabel(e.target.value)} />}
            </Field>
            <div className="rounded-lg border border-line bg-sunken p-3">
              <p className="mb-2 text-sm font-medium text-ink">{t("tgc.consentTitle")}</p>
              <p className="mb-3 text-sm text-ink-soft">{t("tgc.consentText")}</p>
              <Checkbox checked={consent} onChange={setConsent} label={t("tgc.consentAgree")} />
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={close}>
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                loading={start.isPending}
                disabled={!consent || !E164.test(phone.replace(/[\s()-]/g, ""))}
              >
                {t("tgc.sendCode")}
              </Button>
            </div>
          </form>
        )}

        {step === "code" && (
          <form onSubmit={onCode} className="flex flex-col gap-4">
            <p className="text-sm text-ink-soft">{t("tgc.codeHint")}</p>
            <Field label={t("tgc.code")}>
              {(id) => (
                <Input
                  id={id}
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 10))}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  autoFocus
                  className="text-center text-lg tracking-[0.4em]"
                />
              )}
            </Field>
            <div className="flex justify-end">
              <Button type="submit" loading={submitCode.isPending} disabled={code.length < 3}>
                {t("common.confirm")}
              </Button>
            </div>
          </form>
        )}

        {step === "password" && (
          <form onSubmit={onPassword} className="flex flex-col gap-4">
            <p className="text-sm text-ink-soft">{t("tgc.passwordHint")}</p>
            <Field label={t("tgc.password")}>
              {(id) => (
                <Input
                  id={id}
                  type="password"
                  autoComplete="off"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoFocus
                />
              )}
            </Field>
            <div className="flex justify-end">
              <Button type="submit" loading={submitPassword.isPending} disabled={!password}>
                {t("common.confirm")}
              </Button>
            </div>
          </form>
        )}

        {step === "done" && (
          <div className="flex flex-col items-center gap-3 py-4 text-center">
            <CircleCheck className="size-10 text-ok" aria-hidden />
            <p className="font-medium text-ink">{t("tgc.connected")}</p>
            <Button onClick={close}>{t("common.close")}</Button>
          </div>
        )}
      </div>
    </Modal>
  );
}
