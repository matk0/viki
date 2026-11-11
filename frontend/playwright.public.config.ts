import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/public-demo.spec.ts',
  workers: 1,
  timeout: 45000,
  use: { baseURL: 'http://127.0.0.1:8790', channel: 'chrome', headless: true, viewport: { width: 1440, height: 1000 } },
  webServer: {
    command: 'npm --prefix ../public-demo run dev -- --port 8790',
    url: 'http://127.0.0.1:8790/healthz',
    reuseExistingServer: true,
    timeout: 120000,
  },
})
