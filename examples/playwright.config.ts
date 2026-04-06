import { defineConfig } from '@playwright/test';

export default defineConfig({
  timeout: 30_000,
  retries: 0,
  reporter: [['json', { outputFile: undefined }]],
  use: {
    headless: true,
    ignoreHTTPSErrors: true,
  },
});
