import { useI18n } from "../i18n";
import { useCountries, useRegions } from "../lib/queries";
import { Field, Select } from "./ui";

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
