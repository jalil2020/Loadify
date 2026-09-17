import { ArrowLeft, Plus, Trash2 } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router";
import { useErrorText, useToast } from "../../components/feedback";
import { AreaPicker } from "../../components/places";
import { Button, Card, Checkbox, ChipSelect, ErrorState, Field, Input, Notice, PageHeader, Select, Spinner, Textarea } from "../../components/ui";
import { useI18n } from "../../i18n";
import { useCargoTypes, useGroups, useSaveRules, useVehicleTypes } from "../../lib/queries";
import type { SaveGroupRules, TelegramGroup } from "../../lib/types";
import { fieldErrors } from "../../lib/labels";

interface RouteDraft {
  key: number;
  fromCountry: string;
  fromRegion: string;
  toCountry: string;
  toRegion: string;
}

interface Draft {
  language: string;
  postingEnabled: boolean;
  vehicles: string[];
  cargo: string[];
  minWeight: string;
  maxWeight: string;
  hourly: string;
  daily: string;
  cooldown: string;
  quietFrom: string;
  quietTo: string;
  notes: string;
  routes: RouteDraft[];
}

let routeKey = 1;

function toDraft(g: TelegramGroup): Draft {
  const r = g.rules;
  const s = (v: number | null | undefined) => (v === null || v === undefined ? "" : String(v));
  return {
    language: g.language,
    postingEnabled: g.posting_enabled,
    vehicles: r?.vehicle_types ?? [],
    cargo: r?.cargo_type_codes ?? [],
    minWeight: s(r?.min_weight_kg),
    maxWeight: s(r?.max_weight_kg),
    hourly: s(r?.hourly_limit ?? 1),
    daily: s(r?.daily_limit ?? 3),
    cooldown: s(r?.cooldown_minutes ?? 60),
    quietFrom: s(r?.quiet_from_hour),
    quietTo: s(r?.quiet_to_hour),
    notes: r?.notes ?? "",
    routes: (g.routes ?? []).map((rt) => ({
      key: routeKey++,
      fromCountry: rt.origin_country ?? "",
      fromRegion: rt.origin_region_id ?? "",
      toCountry: rt.destination_country ?? "",
      toRegion: rt.destination_region_id ?? "",
    })),
  };
}

const intOrNull = (v: string) => (v.trim() === "" ? null : Math.round(Number(v)));

function toRequest(d: Draft): SaveGroupRules {
  return {
    language: d.language,
    posting_enabled: d.postingEnabled,
    vehicle_types: d.vehicles,
    cargo_type_codes: d.cargo,
    min_weight_kg: intOrNull(d.minWeight),
    max_weight_kg: intOrNull(d.maxWeight),
    hourly_limit: Number(d.hourly),
    daily_limit: Number(d.daily),
    cooldown_minutes: Number(d.cooldown),
    quiet_from_hour: intOrNull(d.quietFrom),
    quiet_to_hour: intOrNull(d.quietTo),
    notes: d.notes.trim() || null,
    routes: d.routes
      .filter((r) => (r.fromCountry || r.fromRegion) && (r.toCountry || r.toRegion))
      .map((r) => ({
        origin_country: r.fromCountry || null,
        origin_region_id: r.fromRegion || null,
        destination_country: r.toCountry || null,
        destination_region_id: r.toRegion || null,
      })),
  };
}

const HOURS = Array.from({ length: 24 }, (_, h) => h);

export function GroupRulesPage() {
  const { id = "" } = useParams();
  const { t } = useI18n();
  const toast = useToast();
  const errorText = useErrorText();
  const groups = useGroups();
  const vehicles = useVehicleTypes();
  const cargoTypes = useCargoTypes();
  const save = useSaveRules(id);
  const group = groups.data?.find((g) => g.id === id);

  const [draft, setDraft] = useState<Draft | null>(null);
  const loaded = useRef<string | null>(null);
  useEffect(() => {
    if (group && loaded.current !== group.id) {
      loaded.current = group.id;
      setDraft(toDraft(group));
    }
  }, [group]);

  if (groups.isLoading) return <Spinner />;
  if (groups.error) return <ErrorState message={errorText(groups.error)} onRetry={() => void groups.refetch()} />;
  if (!group || !draft) return <ErrorState message={t("err.NOT_FOUND")} />;

  const set = <K extends keyof Draft>(key: K, value: Draft[K]) => setDraft((d) => (d ? { ...d, [key]: value } : d));
  const setRoute = (key: number, patch: Partial<RouteDraft>) =>
    set(
      "routes",
      draft.routes.map((r) => (r.key === key ? { ...r, ...patch } : r)),
    );

  const weightsInvalid =
    draft.minWeight !== "" && draft.maxWeight !== "" && Number(draft.minWeight) > Number(draft.maxWeight);
  const quietInvalid = (draft.quietFrom === "") !== (draft.quietTo === "");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (weightsInvalid || quietInvalid) return;
    save.mutate(toRequest(draft), {
      onSuccess: (g) => {
        toast.success(t("rules.saved"));
        loaded.current = null;
        setDraft(toDraft(g));
      },
      onError: (err) => {
        const byField = fieldErrors(err);
        toast.error(Object.keys(byField).length ? `${t("err.VALIDATION_ERROR")}: ${Object.values(byField).join("; ")}` : errorText(err));
      },
    });
  };

  return (
    <>
      <PageHeader
        title={group.title}
        subtitle={t("rules.title")}
        back={
          <Link to="/telegram?tab=groups" className="mb-2 inline-flex items-center gap-1 text-sm text-muted hover:text-ink">
            <ArrowLeft className="size-4" aria-hidden />
            {t("tg.groups")}
          </Link>
        }
      />

      <form onSubmit={submit} className="flex flex-col gap-6">
        {group.slowmode_seconds > 0 && <Notice tone="info">{t("rules.slowmode", { n: group.slowmode_seconds })}</Notice>}

        <Card>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex items-center">
              <Checkbox
                checked={draft.postingEnabled}
                onChange={(v) => set("postingEnabled", v)}
                label={t("rules.posting")}
              />
            </div>
            <Field label={t("rules.language")}>
              {(fid) => (
                <Select id={fid} value={draft.language} onChange={(e) => set("language", e.target.value)}>
                  <option value="uz">O‘zbekcha</option>
                  <option value="ru">Русский</option>
                  <option value="en">English</option>
                </Select>
              )}
            </Field>
          </div>
        </Card>

        <Card title={t("rules.limits")}>
          <div className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-3">
              <Field label={t("rules.hourly")}>
                {(fid) => (
                  <Input id={fid} type="number" min={1} max={10} value={draft.hourly} onChange={(e) => set("hourly", e.target.value)} required />
                )}
              </Field>
              <Field label={t("rules.daily")}>
                {(fid) => (
                  <Input id={fid} type="number" min={1} max={20} value={draft.daily} onChange={(e) => set("daily", e.target.value)} required />
                )}
              </Field>
              <Field label={t("rules.cooldown")}>
                {(fid) => (
                  <Input
                    id={fid}
                    type="number"
                    min={0}
                    max={10080}
                    value={draft.cooldown}
                    onChange={(e) => set("cooldown", e.target.value)}
                    required
                  />
                )}
              </Field>
            </div>
            <p className="text-xs text-muted">{t("rules.limitsHint")}</p>
            <fieldset>
              <legend className="mb-2 text-sm font-medium text-ink-soft">{t("rules.quiet")}</legend>
              <div className="grid gap-4 sm:grid-cols-2">
                {(["quietFrom", "quietTo"] as const).map((key) => (
                  <Field
                    key={key}
                    label={t(key === "quietFrom" ? "rules.quietFrom" : "rules.quietTo")}
                    error={quietInvalid ? t("err.VALIDATION_ERROR") : undefined}
                  >
                    {(fid) => (
                      <Select id={fid} value={draft[key]} onChange={(e) => set(key, e.target.value)}>
                        <option value="">{t("common.none")}</option>
                        {HOURS.map((h) => (
                          <option key={h} value={h}>
                            {String(h).padStart(2, "0")}:00
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                ))}
              </div>
            </fieldset>
          </div>
        </Card>

        <Card title={`${t("rules.vehicles")} · ${t("rules.cargo")}`}>
          <div className="flex flex-col gap-5">
            <div>
              <p className="mb-2 text-sm font-medium text-ink-soft">{t("rules.vehicles")}</p>
              <ChipSelect
                options={(vehicles.data ?? []).map((v) => ({ value: v.code, label: v.name }))}
                value={draft.vehicles}
                onChange={(v) => set("vehicles", v)}
              />
              <p className="mt-2 text-xs text-muted">{t("rules.anyHint")}</p>
            </div>
            <div>
              <p className="mb-2 text-sm font-medium text-ink-soft">{t("rules.cargo")}</p>
              <ChipSelect
                options={(cargoTypes.data ?? []).map((c) => ({ value: c.code, label: c.name }))}
                value={draft.cargo}
                onChange={(v) => set("cargo", v)}
              />
              <p className="mt-2 text-xs text-muted">{t("rules.anyHint")}</p>
            </div>
            <fieldset>
              <legend className="mb-2 text-sm font-medium text-ink-soft">{t("rules.weight")}</legend>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label={t("rules.min")} error={weightsInvalid ? t("err.VALIDATION_ERROR") : undefined}>
                  {(fid) => (
                    <Input id={fid} type="number" min={1} max={100000} value={draft.minWeight} onChange={(e) => set("minWeight", e.target.value)} />
                  )}
                </Field>
                <Field label={t("rules.max")}>
                  {(fid) => (
                    <Input id={fid} type="number" min={1} max={100000} value={draft.maxWeight} onChange={(e) => set("maxWeight", e.target.value)} />
                  )}
                </Field>
              </div>
            </fieldset>
          </div>
        </Card>

        <Card
          title={t("rules.routes")}
          action={
            draft.routes.length < 30 && (
              <Button
                size="sm"
                variant="secondary"
                icon={<Plus className="size-4" />}
                onClick={() =>
                  set("routes", [
                    ...draft.routes,
                    { key: routeKey++, fromCountry: "UZ", fromRegion: "", toCountry: "", toRegion: "" },
                  ])
                }
              >
                {t("rules.addRoute")}
              </Button>
            )
          }
        >
          {draft.routes.length === 0 ? (
            <p className="text-sm text-muted">{t("rules.routesHint")}</p>
          ) : (
            <ul className="flex flex-col gap-4">
              {draft.routes.map((r) => (
                <li key={r.key} className="flex flex-col gap-3 rounded-lg border border-line p-4 lg:flex-row lg:items-end">
                  <AreaPicker
                    label={t("rules.from")}
                    country={r.fromCountry}
                    regionId={r.fromRegion}
                    onChange={(c, reg) => setRoute(r.key, { fromCountry: c, fromRegion: reg })}
                  />
                  <AreaPicker
                    label={t("rules.to")}
                    country={r.toCountry}
                    regionId={r.toRegion}
                    onChange={(c, reg) => setRoute(r.key, { toCountry: c, toRegion: reg })}
                  />
                  <Button
                    variant="ghost"
                    aria-label={t("common.delete")}
                    onClick={() => set("routes", draft.routes.filter((x) => x.key !== r.key))}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card>
          <Field label={t("rules.notes")} optional>
            {(fid) => (
              <Textarea id={fid} rows={3} maxLength={500} value={draft.notes} onChange={(e) => set("notes", e.target.value)} />
            )}
          </Field>
        </Card>

        <div className="flex justify-end">
          <Button type="submit" loading={save.isPending} disabled={weightsInvalid || quietInvalid}>
            {save.isPending ? t("common.saving") : t("common.save")}
          </Button>
        </div>
      </form>
    </>
  );
}
