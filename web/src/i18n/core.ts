export type Lang = "uz" | "ru";
export const LANGS: Lang[] = ["uz", "ru"];

/** Fills {name} placeholders; an unknown name is left visible. */
export function interpolate(template: string, vars?: Record<string, string | number>): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (match, name: string) => (name in vars ? String(vars[name]) : match));
}
