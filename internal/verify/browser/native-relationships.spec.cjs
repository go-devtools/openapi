const { test, expect } = require('./fixtures.cjs');
const document = require('../testdata/browser/native-relationships.json');

// Load the original relationships without a server-side projection of callbacks or webhooks.
async function load(page, source = document) {
 await page.route('**/enabled/docs/openapi.json', route => route.fulfill({ json: source }));
 await page.goto('/enabled/docs/');
 await expect(page.locator('.info .title')).toContainText(document.info.title);
 const notes = page.getByRole('region', { name: 'Swagger UI compatibility' });
 await expect(notes).toBeVisible();
 await notes.locator('summary').click();
 await expect(notes).toContainText('openapi.ui.webhooks');
 return notes;
}

// Preserve callback contracts, response link expressions and server choices alongside the webhook limitation.
test('relationships preserve callback and link metadata and disclose omitted webhooks', async ({ page }, testInfo) => {
 const notes = await load(page);
 await expect(notes).toContainText('#/webhooks/SubscriptionChanged');
 const parent = page.locator('.opblock-post').first();
 await expect(parent.locator('tr.response[data-code="201"]')).toBeVisible();
 await parent.getByText('Callbacks', { exact: true }).click();
 const callback = parent.locator('.opblock').filter({ hasText: 'Subscription event callback' });
 await expect(callback).toContainText('{$request.body#/callbackUrl}');
 await expect(callback.locator('tr.response[data-code="204"]')).toContainText('Event accepted');
 await callback.getByRole('tab', { name: 'Schema', exact: true }).click();
 await expect(callback).toContainText('Event');
 await expect(callback).not.toContainText('_opaque');
 await expect(callback.getByRole('button', { name: 'Execute', exact: true })).toHaveCount(0);
 const links = parent.locator('tr.response[data-code="201"] .response-col_links');
 await expect(links).toContainText('Read the newly created subscription');
 await expect(links).toContainText('getSubscription');
 await expect(links).toContainText('$response.body#/id');
 expect((await links.boundingBox()).width).toBeGreaterThanOrEqual(220);
 await expect(page.getByRole('option', { name: '/ - Local service', exact: true })).toHaveCount(1);
 await expect(page.getByRole('option', { name: '/v2 - Second version', exact: true })).toHaveCount(1);
 await expect(page.locator('.opblock').filter({ hasText: 'Subscription changed webhook' })).toHaveCount(0);
 await page.screenshot({ path: testInfo.outputPath('relationships-desktop.png'), fullPage: true });
 await page.setViewportSize({ width: 390, height: 844 });
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 await expect(notes).toContainText('SubscriptionChanged');
 expect((await links.boundingBox()).width).toBeGreaterThan(200);
 await page.screenshot({ path: testInfo.outputPath('relationships-mobile.png'), fullPage: true });
 await parent.locator('tr.response[data-code="201"]').getByRole('combobox', { name: 'Media Type', exact: true }).selectOption('application/x-ndjson');
 await expect(parent.getByRole('region', { name: 'Response 201 stream item schema' })).toContainText('Event');
 expect((await links.boundingBox()).width).toBeGreaterThan(200);
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 await page.screenshot({ path: testInfo.outputPath('relationships-stream-mobile.png'), fullPage: true });
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(document);
});

// A webhook-only document must describe the omitted contract without inventing a Paths operation.
test('webhook-only documents keep an explicit read-only boundary', async ({ page }) => {
 const source = { openapi: document.openapi, info: document.info,
  webhooks: { SubscriptionChanged: document.components.pathItems.Changed },
  components: { schemas: document.components.schemas } };
 const notes = await load(page, source);
 await expect(notes).toContainText('SubscriptionChanged');
 await expect(page.locator('.opblock')).toHaveCount(0);
 await expect(page.getByRole('button', { name: 'Execute', exact: true })).toHaveCount(0);
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(source);
});
