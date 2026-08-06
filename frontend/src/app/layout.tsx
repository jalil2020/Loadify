import type { Metadata, Viewport } from "next";
import "./globals.css";
import { Providers } from "@/components/providers";
import { AppShell } from "@/components/app-shell";

export const metadata: Metadata = {
  title: {
    default: "Loadify — Yuk bozori",
    template: "%s · Loadify",
  },
  description:
    "Loadify connects logistics companies and truck drivers across Uzbekistan in real time.",
  icons: { icon: "/favicon.svg" },
};

export const viewport: Viewport = {
  // The logo's blue, which is what a mobile browser paints its chrome with.
  // Kept as a literal because a meta tag cannot read a CSS custom property.
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#1E56C9" },
    { media: "(prefers-color-scheme: dark)", color: "#0b1220" },
  ],
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="uz" suppressHydrationWarning>
      <body>
        <Providers>
          <AppShell>{children}</AppShell>
        </Providers>
      </body>
    </html>
  );
}
