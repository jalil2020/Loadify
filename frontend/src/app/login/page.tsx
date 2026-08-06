"use client";

import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useAuth, useLocale } from "@/components/providers";
import { Button, Card, CardBody, Field, Input } from "@/components/ui/primitives";
import { ApiError } from "@/lib/api";

/**
 * The Zod schema mirrors the backend validator, so the obvious mistakes are
 * caught before a request is sent — but the server remains the authority and
 * its field errors are mapped back onto the form.
 */
const schema = z.object({
  phone: z
    .string()
    .regex(/^\+998\d{9}$/, "+998XXXXXXXXX"),
  password: z.string().min(8),
});

type FormValues = z.infer<typeof schema>;

const DEMO_ACCOUNTS = [
  { phone: "+998902222222", role: "company manager" },
  { phone: "+998904444444", role: "driver" },
  { phone: "+998901111111", role: "super admin" },
];

/**
 * useSearchParams opts a route out of static rendering unless it sits behind a
 * Suspense boundary, so the form is split out and the shell prerenders.
 */
export default function LoginPage() {
  return (
    <React.Suspense fallback={<LoginSkeleton />}>
      <LoginForm />
    </React.Suspense>
  );
}

function LoginSkeleton() {
  return (
    <div className="mx-auto max-w-md py-8">
      <Card>
        <CardBody className="space-y-4">
          <div className="skeleton h-6 w-40" />
          <div className="skeleton h-10 w-full" />
          <div className="skeleton h-10 w-full" />
          <div className="skeleton h-10 w-full" />
        </CardBody>
      </Card>
    </div>
  );
}

function LoginForm() {
  const { t } = useLocale();
  const { signIn } = useAuth();
  const router = useRouter();
  const params = useSearchParams();
  const [formError, setFormError] = React.useState<string | null>(null);

  const {
    register,
    handleSubmit,
    setValue,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { phone: "", password: "" },
  });

  const onSubmit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await signIn(values.phone, values.password);
      // `next` comes from our own redirects, so only a same-site path is
      // honoured — an absolute URL here would be an open redirect.
      const next = params.get("next");
      router.push(next?.startsWith("/") ? next : "/");
      router.refresh();
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.isValidation) {
          for (const [field, message] of Object.entries(error.fieldErrors)) {
            setError(field as keyof FormValues, { message });
          }
          return;
        }
        setFormError(error.message);
        return;
      }
      setFormError(t("common.error"));
    }
  });

  return (
    <div className="mx-auto max-w-md py-8">
      <Card>
        <CardBody className="space-y-5">
          <div>
            <h1 className="text-xl font-semibold">{t("auth.signIn")}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{t("auth.signInSubtitle")}</p>
          </div>

          <form onSubmit={onSubmit} className="space-y-4" noValidate>
            <Field label={t("auth.phone")} error={errors.phone?.message}>
              <Input
                {...register("phone")}
                type="tel"
                inputMode="tel"
                autoComplete="username"
                placeholder="+998901234567"
                aria-invalid={Boolean(errors.phone)}
              />
            </Field>

            <Field label={t("auth.password")} error={errors.password?.message}>
              <Input
                {...register("password")}
                type="password"
                autoComplete="current-password"
                aria-invalid={Boolean(errors.password)}
              />
            </Field>

            {formError && (
              <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {formError}
              </p>
            )}

            <Button type="submit" className="w-full" loading={isSubmitting}>
              {isSubmitting ? t("auth.submitting") : t("auth.submit")}
            </Button>
          </form>

          <div className="border-t border-border pt-4">
            <p className="mb-2 text-xs font-medium text-muted-foreground">{t("auth.demoHint")}</p>
            <div className="flex flex-wrap gap-2">
              {DEMO_ACCOUNTS.map((account) => (
                <button
                  key={account.phone}
                  type="button"
                  onClick={() => {
                    setValue("phone", account.phone);
                    setValue("password", "Loadify123");
                  }}
                  className="rounded-md border border-border px-2.5 py-1 text-xs hover:bg-muted"
                >
                  {account.role}
                </button>
              ))}
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
