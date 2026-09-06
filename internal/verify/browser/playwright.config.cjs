const { defineConfig } = require('@playwright/test');
const { mkdtempSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join } = require('node:path');

// Keep browser evidence outside product source and retain traces when a check fails.
module.exports = defineConfig({
 testDir: __dirname,
 testMatch: '**/*.spec.cjs',
 timeout: 60000,
 expect: { timeout: 10000 },
 workers: 1,
 retries: 0,
 reporter: [['list']],
 outputDir: process.env.PLAYWRIGHT_OUTPUT_DIR || mkdtempSync(join(tmpdir(), 'openapi-browser-results-')),
 use: { browserName: 'chromium', headless: true, viewport: { width: 1365, height: 900 }, trace: 'retain-on-failure', screenshot: 'only-on-failure', serviceWorkers: 'block' }
});
