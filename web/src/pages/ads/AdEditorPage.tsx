import { clsx } from "clsx";
import { ArrowLeft, Sparkles, Wand2 } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Controller, useForm } from "react-hook-form";
import { Link, useNavigate, useParams } from "react-router";
import { useErrorText, useToast } from "../../components/feedback";
import { PlacePicker } from "../../components/places";

import {
  Badge,
  Button,
  Card,
  Checkbox,
  ErrorState,
  Field,
  Input,
  LinkButton,
  Notice,
  PageHeader,
  Select,
  Spinner,
  Textarea,
} from "../../components/ui";
import { useI18n, type MessageKey } from "../../i18n";
import { useAd, useCargoTypes, useParseAd, useSaveAd, useVehicleTypes } from "../../lib/queries";
import type { ParsedAdvert } from "../../lib/types";
import { CURRENCIES, emptyAd, fromAd, fromDraft, PAYMENT_METHODS, PAYMENT_TERMS, toRequest, type AdFormValues } from "./adForm";
import { fieldErrors, fieldText } from "../../lib/labels";

const PHONE = /^\+[1-9]\d{7,14}$/;

const aiTone: Record<ParsedAdvert["ai_status"], "neutral" | "ok" | "warn" | "bad" | "info"> = {
  off: "neutral",
  not_needed: "ok",
  used: "info",
  no_result: "neutral",
  limited: "warn",
  unavailable: "bad",
};

/** Where a value came from: the rule or the model, and the words it read. */
function Provenance({ parsed, field }: { parsed: ParsedAdvert | null; field: string }) {
  const { t } = useI18n();
  const p = parsed?.fields[field];
  if (!p?.source || p.value === null || p.value === undefined || p.value === "") return null;
  const ai = p.source === "ai";
  return (
    <span
      className={clsx(
        "inline-flex max-w-full items-center gap-1 truncate rounded-md px-1.5 py-0.5 text-[11px]",
        ai ? "bg-info-soft text-info" : "bg-sunken text-muted",
      )}
      title={p.evidence}
    >
      {ai && <Sparkles className="size-3 shrink-0" aria-hidden />}
      {t(ai ? "ed.source.ai" : "ed.source.rule")}
      {p.evidence && <span className="truncate">· {t("ed.from", { text: p.evidence })}</span>}
    </span>
  );
}

function Label({ text, parsed, field }: { text: string; parsed: ParsedAdvert | null; field?: string }) {
  return (
    <span className="flex flex-wrap items-center gap-2">
      {text}
      {field && <Provenance parsed={parsed} field={field} />}
    </span>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-4">
      <h3 className="text-sm font-semibold tracking-wide text-muted uppercase">{title}</h3>
      {children}
    </section>
  );
}

export function AdEditorPage() {
  const { id } = useParams();
  const editing = Boolean(id);
  const { t } = useI18n();
  const navigate = useNavigate();
  const toast = useToast();
  const errorText = useErrorText();

  const existing = useAd(id);
  const parse = useParseAd();
  const save = useSaveAd(id);
  const vehicles = useVehicleTypes();
  const cargoTypes = useCargoTypes();

  const [parsed, setParsed] = useState<ParsedAdvert | null>(null);
  const [manual, setManual] = useState(false);
  const loadedId = useRef<string | null>(null);

  const form = useForm<AdFormValues>({ defaultValues: emptyAd });
  const { register, control, formState } = form;
  const errors = formState.errors;

  // Fill the form once per advert; a background refetch must not wipe edits.
  useEffect(() => {
    if (existing.data && loadedId.current !== existing.data.id) {
      loadedId.current = existing.data.id;
      form.reset(fromAd(existing.data));
    }
  }, [existing.data, form]);

  const read = () => {
    const text = form.getValues("raw_text").trim();
    if (text.length < 3) return;
    parse.mutate(text, {
      onSuccess: (res) => {
        form.reset(fromDraft({ ...res.draft, raw_text: text }));
        setParsed(res);
      },
      onError: (err) => toast.error(errorText(err)),
    });
  };

  const submit = form.handleSubmit((values) => {
    save.mutate(toRequest(values), {
      onSuccess: (ad) => {
        toast.success(t("ed.saved"));
        navigate(`/ads/${ad.id}`);
      },
      onError: (err) => {
        const byField = fieldErrors(err);
        if (Object.keys(byField).length === 0) toast.error(errorText(err));
        else toast.error(`${t("err.VALIDATION_ERROR")}: ${Object.keys(byField).join(", ")}`);
      },
    });
  });

  if (editing && existing.isLoading) return <Spinner />;
  if (editing && existing.error) return <ErrorState message={errorText(existing.error)} onRetry={() => void existing.refetch()} />;

  const showForm = editing || parsed !== null || manual;

  return (
    <>
      <PageHeader
        title={t(editing ? "ed.editTitle" : "ed.newTitle")}
        back={
          <Link to={editing ? `/ads/${id}` : "/ads"} className="mb-2 inline-flex items-center gap-1 text-sm text-muted hover:text-ink">
            <ArrowLeft className="size-4" aria-hidden />
            {t("common.back")}
          </Link>
        }
      />

      <form onSubmit={submit} className="flex flex-col gap-6" noValidate>
        <Card>
          <Field label={t("ed.textLabel")} hint={t("ed.textHint")}>
            {(fieldId) => (
              <Textarea
                id={fieldId}
                rows={editing ? 4 : 6}
                maxLength={4000}
                placeholder={t("ed.textPlaceholder")}
                {...register("raw_text")}
              />
            )}
          </Field>
          {!editing && (
            <div className="mt-4 flex flex-wrap items-center gap-3">
              <Button onClick={read} loading={parse.isPending} icon={<Wand2 className="size-4" />}>
                {parse.isPending ? t("ed.reading") : t("ed.read")}
              </Button>
              {!showForm && (
                <button type="button" onClick={() => setManual(true)} className="text-sm text-muted hover:text-ink">
                  {t("ed.skipRead")}
                </button>
              )}
            </div>
          )}

          {parsed && (
            <div className="mt-5 flex flex-col gap-3 border-t border-line pt-4">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-ink">{t("ed.review")}</span>
                <Badge tone={aiTone[parsed.ai_status]} icon={<Sparkles className="size-3" aria-hidden />}>
                  {t(`ed.ai.${parsed.ai_status}` as MessageKey)}
                </Badge>
              </div>
              <p className="text-xs text-muted">{t("ed.reviewHint")}</p>
              {parsed.missing.length > 0 && (
                <Notice tone="warn">
                  {t("ed.missing")}: {parsed.missing.map((f) => fieldText(f, t)).join(", ")}
                </Notice>
              )}
              {parsed.dropped.length > 0 && (
                <div className="flex flex-wrap items-center gap-1.5 text-sm">
                  <span className="text-muted">{t("ed.notUnderstood")}:</span>
                  {parsed.dropped.map((word) => (
                    <Badge key={word}>{word}</Badge>
                  ))}
                </div>
              )}
            </div>
          )}
        </Card>

        {showForm && (
          <>
            <Card>
              <Section title={t("ed.sectionRoute")}>
                <Controller
                  control={control}
                  name="origin"
                  render={({ field }) => (
                    <div>
                      <Provenance parsed={parsed} field="origin" />
                      <PlacePicker legend={t("field.origin")} value={field.value} onChange={field.onChange} />
                    </div>
                  )}
                />
                <div className="border-t border-line" />
                <Controller
                  control={control}
                  name="destination"
                  render={({ field }) => (
                    <div>
                      <Provenance parsed={parsed} field="destination" />
                      <PlacePicker legend={t("field.destination")} value={field.value} onChange={field.onChange} />
                    </div>
                  )}
                />
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label={<Label text={t("field.loading_date")} parsed={parsed} field="loading_date" />}>
                    {(fieldId) => <Input id={fieldId} type="date" {...register("loading_date")} />}
                  </Field>
                  <Field label={t("field.loading_date_text")} optional>
                    {(fieldId) => <Input id={fieldId} maxLength={64} {...register("loading_date_text")} />}
                  </Field>
                </div>
              </Section>
            </Card>

            <Card>
              <Section title={t("ed.sectionCargo")}>
                <div className="grid gap-4 sm:grid-cols-3">
                  <Field
                    label={<Label text={t("field.weight_t")} parsed={parsed} field="weight_kg" />}
                    error={errors.weight_t && t("err.VALIDATION_ERROR")}
                  >
                    {(fieldId) => (
                      <Input
                        id={fieldId}
                        inputMode="decimal"
                        invalid={!!errors.weight_t}
                        {...register("weight_t", {
                          validate: (v) => v === "" || (Number(v.replace(",", ".")) > 0 && Number(v.replace(",", ".")) <= 100),
                        })}
                      />
                    )}
                  </Field>
                  <Field
                    label={<Label text={t("field.volume_m3")} parsed={parsed} field="volume_m3" />}
                    error={errors.volume_m3 && t("err.VALIDATION_ERROR")}
                  >
                    {(fieldId) => (
                      <Input
                        id={fieldId}
                        inputMode="decimal"
                        invalid={!!errors.volume_m3}
                        {...register("volume_m3", {
                          validate: (v) => v === "" || (Number(v.replace(",", ".")) > 0 && Number(v.replace(",", ".")) <= 200),
                        })}
                      />
                    )}
                  </Field>
                  <Field label={<Label text={t("field.vehicle_type")} parsed={parsed} field="vehicle_type" />}>
                    {(fieldId) => (
                      <Select id={fieldId} {...register("vehicle_type")}>
                        <option value="">{t("common.any")}</option>
                        {vehicles.data?.map((v) => (
                          <option key={v.code} value={v.code}>
                            {v.name}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label={<Label text={t("field.cargo_type")} parsed={parsed} field="cargo_type" />}>
                    {(fieldId) => (
                      <Select id={fieldId} {...register("cargo_type_id")}>
                        <option value="">{t("common.none")}</option>
                        {cargoTypes.data?.map((c) => (
                          <option key={c.id} value={c.id}>
                            {c.name}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                  <Field label={t("field.cargo_text")} optional>
                    {(fieldId) => <Input id={fieldId} maxLength={120} {...register("cargo_text")} />}
                  </Field>
                </div>
              </Section>
            </Card>

            <Card>
              <Section title={t("ed.sectionPrice")}>
                <div className="grid gap-4 sm:grid-cols-3">
                  <Field
                    label={<Label text={t("field.price")} parsed={parsed} field="price" />}
                    error={errors.price && t("err.VALIDATION_ERROR")}
                  >
                    {(fieldId) => (
                      <Input
                        id={fieldId}
                        inputMode="decimal"
                        invalid={!!errors.price}
                        {...register("price", {
                          validate: (v) => v === "" || Number(v.replace(",", ".").replace(/\s/g, "")) >= 0,
                        })}
                      />
                    )}
                  </Field>
                  <Field label={t("field.currency")}>
                    {(fieldId) => (
                      <Select id={fieldId} {...register("price_currency")}>
                        {CURRENCIES.map((c) => (
                          <option key={c} value={c}>
                            {c}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                  <div className="flex items-end pb-2">
                    <Controller
                      control={control}
                      name="price_negotiable"
                      render={({ field }) => (
                        <Checkbox checked={field.value} onChange={field.onChange} label={t("field.negotiable")} />
                      )}
                    />
                  </div>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label={<Label text={t("field.payment_terms")} parsed={parsed} field="payment_terms" />}>
                    {(fieldId) => (
                      <Select id={fieldId} {...register("payment_terms")}>
                        <option value="">{t("common.none")}</option>
                        {PAYMENT_TERMS.map((p) => (
                          <option key={p} value={p}>
                            {t(`pay.${p}`)}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                  <Field label={t("field.payment_method")}>
                    {(fieldId) => (
                      <Select id={fieldId} {...register("payment_method")}>
                        <option value="">{t("common.none")}</option>
                        {PAYMENT_METHODS.map((p) => (
                          <option key={p} value={p}>
                            {t(`pm.${p}`)}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                </div>
              </Section>
            </Card>

            <Card>
              <Section title={t("ed.sectionContact")}>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field
                    label={<Label text={t("field.contact_phone")} parsed={parsed} field="contact_phone" />}
                    error={errors.contact_phone && "+998901234567"}
                  >
                    {(fieldId) => (
                      <Input
                        id={fieldId}
                        type="tel"
                        placeholder="+998901234567"
                        invalid={!!errors.contact_phone}
                        {...register("contact_phone", {
                          validate: (v) => v.trim() === "" || PHONE.test(v.replace(/[\s()-]/g, "")),
                        })}
                      />
                    )}
                  </Field>
                  <Field label={t("field.contact_name")} optional>
                    {(fieldId) => <Input id={fieldId} maxLength={120} {...register("contact_name")} />}
                  </Field>
                </div>
                <Field label={t("field.notes")} optional>
                  {(fieldId) => <Textarea id={fieldId} rows={3} maxLength={2000} {...register("notes")} />}
                </Field>
              </Section>
            </Card>

            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-xs text-muted">{t("ed.draftHint")}</p>
              <div className="flex gap-2">
                <LinkButton to={editing ? `/ads/${id}` : "/ads"} variant="secondary">
                  {t("common.cancel")}
                </LinkButton>
                <Button type="submit" loading={save.isPending}>
                  {save.isPending ? t("common.saving") : t("common.save")}
                </Button>
              </div>
            </div>
          </>
        )}
      </form>
    </>
  );
}
