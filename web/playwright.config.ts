import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  workers: 1,
  use: {
    baseURL: process.env.PAC_BASE_URL || 'http://127.0.0.1:8080',
    trace: 'retain-on-failure',
    launchOptions: process.env.PAC_CHROMIUM_PATH ? { executablePath: process.env.PAC_CHROMIUM_PATH } : undefined,
  },
})
