import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The API is proxied in development, so the browser talks to one origin and
// no CORS setup is needed. In production the built files are served behind the
// same reverse proxy as /api.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
    proxy: {
      "/api": { target: process.env.LOADIFY_API ?? "http://localhost:8080", changeOrigin: true },
    },
  },
  build: {
    rolldownOptions: {
      output: {
        // Libraries change far less often than the app, so they get their own
        // long-cached chunks.
        codeSplitting: {
          groups: [
            { name: "react", test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/, priority: 30 },
            { name: "router", test: /node_modules[\\/]react-router[\\/]/, priority: 20 },
            { name: "data", test: /node_modules[\\/](@tanstack|react-hook-form|@hookform|zod)[\\/]/, priority: 20 },
            { name: "vendor", test: /node_modules[\\/]/, priority: 10 },
          ],
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
  },
});
