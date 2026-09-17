import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Link } from "react-router";
import * as z from "zod";
import { useErrorText } from "../../components/feedback";
import { Button, Field, Input, Notice } from "../../components/ui";
import { useI18n } from "../../i18n";
import { useAuth } from "../../lib/auth";
import { normalizePhone, UZ_PHONE } from "../../lib/labels";
import { AuthLayout } from "./AuthLayout";

const schema = z.object({
  phone: z.string().transform(normalizePhone).pipe(z.string().regex(UZ_PHONE)),
  password: z.string().min(1),
});

type Values = z.input<typeof schema>;

export function LoginPage() {
  const { t } = useI18n();
  const { login } = useAuth();
  const errorText = useErrorText();
  const [error, setError] = useState<string | null>(null);
  const form = useForm<Values, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { phone: "+998", password: "" },
  });

  const submit = form.handleSubmit(async (values) => {
    setError(null);
    try {
      await login(values.phone, values.password);
    } catch (err) {
      setError(errorText(err));
    }
  });

  return (
    <AuthLayout title={t("auth.login.title")} subtitle={t("auth.login.subtitle")}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {error && <Notice tone="bad">{error}</Notice>}
        <Field label={t("auth.phone")} error={form.formState.errors.phone && t("form.phone")}>
          {(id) => (
            <Input
              id={id}
              type="tel"
              autoComplete="username"
              inputMode="tel"
              invalid={Boolean(form.formState.errors.phone)}
              {...form.register("phone")}
            />
          )}
        </Field>
        <Field label={t("auth.password")}>
          {(id) => <Input id={id} type="password" autoComplete="current-password" {...form.register("password")} />}
        </Field>
        <Button type="submit" loading={form.formState.isSubmitting} className="w-full">
          {t("auth.login.submit")}
        </Button>
      </form>
      <p className="mt-6 text-center text-sm text-muted">
        {t("auth.login.noAccount")}{" "}
        <Link to="/register" className="font-medium text-brand hover:underline">
          {t("auth.register.link")}
        </Link>
      </p>
    </AuthLayout>
  );
}
