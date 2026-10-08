import { defineConfig } from '@playwright/test';
export default defineConfig({ testDir: '.', testMatch: '*.spec.ts', workers: 1, retries: 0, timeout: 180_000, reporter: 'line', use: { trace: 'retain-on-failure' } });
