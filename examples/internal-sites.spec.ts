import { test, expect } from '@playwright/test';

test('localhost:3000 loads successfully', async ({ page }) => {
  await page.goto('http://localhost:3000');
  await expect(page.locator('body')).not.toBeEmpty();
});

test('node exporter metrics endpoint responds', async ({ page }) => {
  await page.goto('http://localhost:9100/metrics');
  await expect(page.locator('body')).toContainText('node_');
});
