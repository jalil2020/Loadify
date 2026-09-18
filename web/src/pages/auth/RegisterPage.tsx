import { zodResolver } from "@hookform/resolvers/zod";
import { useState, type FormEvent } from "react";
import { useForm } from "react-hook-form";
import { Link } from "react-router";
import * as z from "zod";
import { useErrorText, useToast } from "../../components/feedback";
import { Button, Field, Input, Notice } from "../../components/ui";
import { useI18n } from "../../i18n";
import { useAuth } from "../../lib/auth";
import { fieldErrors, normalizePhone, UZ_PHONE } from "../../lib/labels";
import { AuthLayout } from "./AuthLayout";

const PASSWORD = /^(?=.*[A-Za-z])(?=.*\d).{8,72}$/;

const schema = z.object({
  company: z.string().trim().min(2).max(255),
  phone: z.string().transform(normalizePhone).pipe(z.string().regex(UZ_PHONE)),
  password: z.string().regex(PASSWORD),
});

type Values = z.input<typeof schema>;

/**
 * Company name, phone and password, and the person is in. The phone number is
 * the login. An SMS step appears only when the server is configured to ask
 * for one.
 */
export function RegisterPage() {
  const { t, lang } = useI18n();
  const { register } = useAuth();
  const errorText = useErrorText();
  const [error, setError] = useState<string | null>(null);
  const [verifyPhone, setVerifyPhone] = useState<string | null>(null);
  const form = useForm<Values, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { company: "", phone: "+998", password: "" },
  });
  const errors = form.formState.errors;

  const submit = form.handleSubmit(async (values) => {
    setError(null);
    try {
      const { verificationRequired } = await register({ ...values, locale: lang });
      if (verificationRequired) setVerifyPhone(values.phone);
    } catch (err) {
      const byField = fieldErrors(err);
      if (byField.phone) form.setError("phone", { message: byField.phone });
      if (byField.password) form.setError("password", { message: byField.password });
      setError(errorText(err));
    }
  });

  if (verifyPhone) return <VerifyStep phone={verifyPhone} onBack={() => setVerifyPhone(null)} />;

  return (
    <AuthLayout title={t("auth.register.title")} subtitle={t("auth.register.subtitle")}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {error && <Notice tone="bad">{error}</Notice>}
        <Field label={t("auth.companyName")} error={errors.company && t("form.tooShort")}>
          {(id) => (
            <Input id={id} autoComplete="organization" invalid={!!errors.company} {...form.register("company")} />
          )}
        </Field>
        <Field label={t("auth.phone")} hint={t("auth.phoneIsLogin")} error={errors.phone && t("form.phone")}>
          {(id) => (
            <Input
              id={id}
              type="tel"
              inputMode="tel"
              autoComplete="username"
              invalid={!!errors.phone}
              {...form.register("phone")}
            />
          )}
        </Field>
        <Field label={t("auth.password")} hint={t("auth.passwordHint")} error={errors.password && t("auth.passwordHint")}>
          {(id) => (
            <Input
              id={id}
              type="password"
              autoComplete="new-password"
              invalid={!!errors.password}
              {...form.register("password")}
            />
          )}
        </Field>
        <Button type="submit" loading={form.formState.isSubmitting} className="w-full">
          {t("auth.register.submit")}
        </Button>
      </form>
      <p className="mt-6 text-center text-sm text-muted">
        {t("auth.haveAccount")}{" "}
        <Link to="/login" className="font-medium text-brand hover:underline">
          {t("auth.login.submit")}
        </Link>
      </p>
    </AuthLayout>
  );
}

function VerifyStep({ phone, onBack }: { phone: string; onBack: () => void }) {
  const { t } = useI18n();
  const { verify, resendCode } = useAuth();
  const errorText = useErrorText();
  const toast = useToast();
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [resending, setResending] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await verify(phone, code.trim());
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  const resend = async () => {
    setResending(true);
    try {
      await resendCode(phone);
      toast.success(t("auth.verify.resent"));
    } catch (err) {
      setError(errorText(err));
    } finally {
      setResending(false);
    }
  };

  return (
    <AuthLayout title={t("auth.verify.title")} subtitle={t("auth.verify.sent", { phone })}>
      <form onSubmit={submit} className="flex flex-col gap-4">
        {error && <Notice tone="bad">{error}</Notice>}
        <Field label={t("auth.verify.code")}>
          {(id) => (
            <Input
              id={id}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 8))}
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              className="text-center text-lg tracking-[0.4em]"
            />
          )}
        </Field>
        <Button type="submit" loading={busy} disabled={code.length < 4} className="w-full">
          {t("auth.verify.submit")}
        </Button>
        <div className="flex justify-between text-sm">
          <button type="button" onClick={onBack} className="text-muted hover:text-ink">
            {t("common.back")}
          </button>
          <button type="button" onClick={() => void resend()} disabled={resending} className="text-brand hover:underline">
            {t("auth.verify.resend")}
          </button>
        </div>
      </form>
    </AuthLayout>
  );
}
