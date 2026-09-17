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
  first_name: z.string().trim().min(2).max(100),
  last_name: z.string().trim().min(2).max(100),
  phone: z.string().transform(normalizePhone).pipe(z.string().regex(UZ_PHONE)),
  password: z.string().regex(PASSWORD),
});

type Values = z.input<typeof schema>;

export function RegisterPage() {
  const { t, lang } = useI18n();
  const { register } = useAuth();
  const errorText = useErrorText();
  const [error, setError] = useState<string | null>(null);
  const [phone, setPhone] = useState<string | null>(null);
  const form = useForm<Values, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { first_name: "", last_name: "", phone: "+998", password: "" },
  });
  const errors = form.formState.errors;

  const submit = form.handleSubmit(async (values) => {
    setError(null);
    try {
      await register({ ...values, locale: lang });
      setPhone(values.phone);
    } catch (err) {
      const byField = fieldErrors(err);
      for (const [field, message] of Object.entries(byField)) {
        if (field in values) form.setError(field as keyof Values, { message });
      }
      setError(errorText(err));
    }
  });

  if (phone) return <VerifyStep phone={phone} onBack={() => setPhone(null)} />;

  return (
    <AuthLayout title={t("auth.register.title")} subtitle={t("auth.register.subtitle")}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {error && <Notice tone="bad">{error}</Notice>}
        <div className="grid grid-cols-2 gap-3">
          <Field label={t("auth.firstName")} error={errors.first_name && t("form.tooShort")}>
            {(id) => (
              <Input id={id} autoComplete="given-name" invalid={!!errors.first_name} {...form.register("first_name")} />
            )}
          </Field>
          <Field label={t("auth.lastName")} error={errors.last_name && t("form.tooShort")}>
            {(id) => (
              <Input id={id} autoComplete="family-name" invalid={!!errors.last_name} {...form.register("last_name")} />
            )}
          </Field>
        </div>
        <Field label={t("auth.phone")} error={errors.phone && t("form.phone")}>
          {(id) => (
            <Input id={id} type="tel" inputMode="tel" autoComplete="tel" invalid={!!errors.phone} {...form.register("phone")} />
          )}
        </Field>
        <Field
          label={t("auth.password")}
          hint={t("auth.passwordHint")}
          error={errors.password && t("auth.passwordHint")}
        >
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
