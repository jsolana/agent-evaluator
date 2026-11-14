import { defineConfig } from "@playwright/test";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Both servers are started for the run: the Go backend with a deterministic
// fake agent and judge (no credentials, fresh database) and the production
// bundle served by `vite preview` (same /api proxy, no dev-server reloads).
const API_PORT = 18180, UI_PORT = 5174;

export default defineConfig({
  testDir: "e2e",
  timeout: 90_000,
  expect: { timeout: 15_000 },
  use: { baseURL: `http://localhost:${UI_PORT}`, trace: "retain-on-failure" },
  webServer: [
    {
      command: "go run ./cmd/server",
      cwd: "../backend",
      url: `http://localhost:${API_PORT}/api/health`,
      env: { AGENT: "fake", JUDGE_PROVIDER: "fake", PORT: String(API_PORT), DB_PATH: join(tmpdir(), `e2e-${Date.now()}.db`), SUITE_MIN_TURNS: "3" },
      timeout: 120_000,
    },
    {
      command: `npx vite build && npx vite preview --port ${UI_PORT} --strictPort`,
      url: `http://localhost:${UI_PORT}`,
      env: { VITE_API_TARGET: `http://localhost:${API_PORT}` },
      timeout: 120_000,
    },
  ],
});
