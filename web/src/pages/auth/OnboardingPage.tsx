import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import * as z from "zod";
import { useErrorText } from "../../components/feedback";
import { Button, Field, Input, Notice } from "../../components/ui";
import { useI18n } from "../../i18n";
import { useAuth } from "../../lib/auth";
import { normalizePhone, UZ_PHONE } from "../../lib/labels";
import { AuthLayout } from "./AuthLayout";

const schema = z.object({
  legal_name: z.string().trim().min(2).max(255),
  brand_name: z.string().trim().min(2).max(255),
  phone: z.string().transform(normalizePhone).pipe(z.string().regex(UZ_PHONE)),
});

type Values = z.input<typeof schema>;

export function OnboardingPage() {
  const { t } = useI18n();
  const { user, createCompany, logout } = useAuth();
  const errorText = useErrorText();
  const [error, setError] = useState<string | null>(null);
  const form = useForm<Values, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { legal_name: "", brand_name: "", phone: user?.phone ?? "+998" },
  });
  const errors = form.formState.errors;

  const submit = form.handleSubmit(async (values) => {
    setError(null);
    try {
      await createCompany(values);
    } catch (err) {
      setError(errorText(err));
    }
  });

  return (
    <AuthLayout title={t("onb.title")} subtitle={t("onb.subtitle")}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {error && <Notice tone="bad">{error}</Notice>}
        <Field label={t("onb.brandName")} error={errors.brand_name && t("form.tooShort")}>
          {(id) => <Input id={id} invalid={!!errors.brand_name} {...form.register("brand_name")} />}
        </Field>
        <Field label={t("onb.legalName")} error={errors.legal_name && t("form.tooShort")}>
          {(id) => <Input id={id} invalid={!!errors.legal_name} {...form.register("legal_name")} />}
        </Field>
        <Field label={t("onb.phone")} error={errors.phone && t("form.phone")}>
          {(id) => <Input id={id} type="tel" invalid={!!errors.phone} {...form.register("phone")} />}
        </Field>
        <Button type="submit" loading={form.formState.isSubmitting} className="w-full">
          {t("onb.submit")}
        </Button>
        <button type="button" onClick={() => void logout()} className="text-sm text-muted hover:text-ink">
          {t("nav.logout")}
        </button>
      </form>
    </AuthLayout>
  );
}
