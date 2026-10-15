import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: 'list',
  outputDir: '../../.tools/playwright-results',
  use: {
    baseURL: 'http://localhost:3007',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: [
    {
      command: 'node ../../scripts/e2e_server.mjs',
      gracefulShutdown: { signal: 'SIGTERM', timeout: 10000 },
      url: 'http://localhost:8087/api/v1/health/ready',
      timeout: 60000,
      reuseExistingServer: false,
    },
    {
      command: 'node .output/server/index.mjs',
      url: 'http://localhost:3007/login',
      env: {
        HOST: '127.0.0.1',
        PORT: '3007',
        NUXT_PUBLIC_API_BASE: 'http://localhost:8087/api/v1',
      },
      timeout: 30000,
      reuseExistingServer: false,
    },
  ],
})
