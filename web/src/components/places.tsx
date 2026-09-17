import { useI18n } from "../i18n";
import { useCities, useCountries, useRegions } from "../lib/queries";
import type { UUID } from "../lib/types";
import { Field, Input, Select } from "./ui";

export interface PlaceValue {
  country: string;
  region_id: UUID;
  city_id: UUID;
  text: string;
}

/**
 * Country → region → city, plus the words as written. Changing a level clears
 * the ones below it, so a Moscow city id never sits under Uzbekistan.
 */
export function PlacePicker({
  legend,
  value,
  onChange,
  withText = true,
}: {
  legend: string;
  value: PlaceValue;
  onChange: (value: PlaceValue) => void;
  withText?: boolean;
}) {
  const { t } = useI18n();
  const countries = useCountries();
  const regions = useRegions(value.country || undefined);
  const cities = useCities(value.region_id || undefined);

  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-1 text-sm font-semibold text-ink">{legend}</legend>
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label={t("field.country")}>
          {(id) => (
            <Select
              id={id}
              value={value.country}
              onChange={(e) => onChange({ ...value, country: e.target.value, region_id: "", city_id: "" })}
            >
              <option value="">{t("common.none")}</option>
              {countries.data?.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.flag_emoji ? `${c.flag_emoji} ` : ""}
                  {c.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label={t("field.region")}>
          {(id) => (
            <Select
              id={id}
              value={value.region_id}
              disabled={!value.country}
              onChange={(e) => onChange({ ...value, region_id: e.target.value, city_id: "" })}
            >
              <option value="">{t("common.none")}</option>
              {regions.data?.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label={t("field.city")}>
          {(id) => (
            <Select
              id={id}
              value={value.city_id}
              disabled={!value.region_id}
              onChange={(e) => onChange({ ...value, city_id: e.target.value })}
            >
              <option value="">{t("common.none")}</option>
              {cities.data?.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
      </div>
      {withText && (
        <Field label={t("field.placeText")} optional>
          {(id) => (
            <Input id={id} value={value.text} maxLength={120} onChange={(e) => onChange({ ...value, text: e.target.value })} />
          )}
        </Field>
      )}
    </fieldset>
  );
}

/** A country, and optionally one of its regions — what a group route names. */
export function AreaPicker({
  label,
  country,
  regionId,
  onChange,
}: {
  label: string;
  country: string;
  regionId: string;
  onChange: (country: string, regionId: string) => void;
}) {
  const { t } = useI18n();
  const countries = useCountries();
  const regions = useRegions(country || undefined);
  return (
    <div className="grid flex-1 gap-2 sm:grid-cols-2">
      <Field label={label}>
        {(id) => (
          <Select id={id} value={country} onChange={(e) => onChange(e.target.value, "")}>
            <option value="">{t("common.none")}</option>
            {countries.data?.map((c) => (
              <option key={c.code} value={c.code}>
                {c.flag_emoji ? `${c.flag_emoji} ` : ""}
                {c.name}
              </option>
            ))}
          </Select>
        )}
      </Field>
      <Field label={t("field.region")}>
        {(id) => (
          <Select id={id} value={regionId} disabled={!country} onChange={(e) => onChange(country, e.target.value)}>
            <option value="">{t("rules.wholeCountry")}</option>
            {regions.data?.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </Select>
        )}
      </Field>
    </div>
  );
}
