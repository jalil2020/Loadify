import { dirname } from "path";
import { fileURLToPath } from "url";
import { FlatCompat } from "@eslint/eslintrc";

const compat = new FlatCompat({
  baseDirectory: dirname(fileURLToPath(import.meta.url)),
});

/**
 * eslint-config-next still ships as eslintrc, so it is bridged through
 * FlatCompat rather than rewritten — the shareable config is the thing that
 * knows about the App Router's rules, and reimplementing it here would drift.
 */
const config = [
  {
    ignores: [".next/**", "node_modules/**", "next-env.d.ts"],
  },
  ...compat.extends("next/core-web-vitals", "next/typescript"),
  {
    rules: {
      // The API returns snake_case and money as minor-unit integers; an unused
      // destructured field is usually deliberate documentation of the shape.
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
    },
  },
];

export default config;
