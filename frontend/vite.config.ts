import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// The dev server proxies /api to the Go backend, so the browser talks to a
// single origin and no CORS is needed during development (preview reuses it).
export default defineConfig({
  plugins: [react()],
  server: { proxy: { "/api": process.env.VITE_API_TARGET ?? "http://localhost:8080" } },
  test: { environment: "jsdom", globals: true, setupFiles: ["src/test-setup.ts"], include: ["src/**/*.test.{ts,tsx}"] },
});
