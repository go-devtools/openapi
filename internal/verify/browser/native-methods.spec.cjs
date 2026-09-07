const { test, expect } = require('./fixtures.cjs');

// Read the current document through the shared compatibility function without changing browser state.
async function report(page) {
 return page.evaluate(() => window.ui.fn.openapiUICompatibility(window.ui.specSelectors.specJson().toJS()));
}

// Preserve native operations and metadata even when the pinned interactive renderer omits them.
test('native QUERY is visible and unrendered methods and tags have explicit diagnostics', async ({ page, request }, testInfo) => {
 const businessRequests = [];
 page.on('request', request => {
  if (new URL(request.url()).pathname === '/query-submit') businessRequests.push(request.method());
 });
 await page.goto('/methods/docs/');
 await expect(page.locator('.info .title')).toContainText('Native methods API');
 await expect(page.locator('.opblock-query')).toBeVisible();
 await expect(page.locator('.opblock')).toHaveCount(2);
 await expect(page.getByRole('button', { name: 'Try it out', exact: true })).toHaveCount(0);
 await expect(page.getByRole('button', { name: 'Execute', exact: true })).toHaveCount(0);
 const notes = page.getByRole('region', { name: 'Swagger UI compatibility' });
 await expect(notes).toBeVisible();
 await notes.locator('summary').click();
 await expect(notes).toContainText('SEARCH /native-methods');
 await expect(notes).toContainText('search /native-methods');
 await expect(notes).toContainText('summary: "Catalog search", parent: "Platform", kind: "nav"');
 const diagnostics = (await report(page)).diagnostics;
 expect(diagnostics.map(d => d.code)).toEqual([
  'openapi.ui.additionalOperations', 'openapi.ui.additionalOperations', 'openapi.ui.additionalOperations',
  'openapi.ui.tagMetadata', 'openapi.ui.tagMetadata'
 ]);
 expect(diagnostics[0]).toMatchObject({ severity: 'warning', route: 'SEARCH /native-methods',
  source: { rule: 'swaggerui/5.32.15', kind: 'ui-limitation' } });
 expect(diagnostics[0].message).toContain('#/paths/~1native-methods/additionalOperations/SEARCH:');
 expect(diagnostics.every(d => d.fix.length > 0)).toBe(true);
 const original = await (await request.get('/methods/docs/openapi.json')).json();
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(original);
 expect(original.openapi).toBe('3.2.0');
 expect(Object.keys(original.paths['/native-methods'].additionalOperations)).toEqual(['SEARCH', 'search']);
 await expect(notes).toContainText('REPORT /linked-method');
 await expect(notes).toContainText('<img src=x onerror=alert(1)>');
 await expect(notes.locator('img')).toHaveCount(0);
 expect(diagnostics[2].message).toContain('#/components/pathItems/LinkedMethods/additionalOperations/REPORT:');
 expect(businessRequests).toEqual([]);
 await page.screenshot({ path: testInfo.outputPath('native-methods-desktop.png'), fullPage: true });
 await page.setViewportSize({ width: 390, height: 844 });
 await expect(notes).toBeVisible();
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
 await page.screenshot({ path: testInfo.outputPath('native-methods-mobile.png'), fullPage: true });
});

// Send exact QUERY bytes only after explicit method configuration; other methods stay disabled.
test('explicit QUERY submission preserves method case and request bytes', async ({ page }) => {
 await page.goto('/methods-enabled/docs/');
 const operation = page.locator('.opblock-query');
 await expect(operation).toBeVisible();
 const body = '{ "term": "stars & 星辰", "limit": 9007199254740993 }';
 await operation.locator('textarea').fill(body);
 const received = page.waitForResponse(response => response.url().endsWith('/query-submit'));
 await operation.getByRole('button', { name: 'Execute', exact: true }).click();
 const response = await received;
 expect(response.status()).toBe(200);
 expect(response.request().method()).toBe('QUERY');
 expect(response.request().postData()).toBe(body);
 expect(await response.json()).toEqual({ body: body, method: 'QUERY' });
 await expect(page.locator('.opblock-get').getByRole('button', { name: 'Execute', exact: true })).toHaveCount(0);
 await expect(page.locator('.opblock-get').getByRole('button', { name: 'Try it out', exact: true })).toHaveCount(0);
});

// Definition changes must refresh the diagnostics rather than retaining stale warnings from another document.
test('native display diagnostics follow the selected document', async ({ page }) => {
 await page.goto('/methods/docs/');
 const notes = page.getByRole('region', { name: 'Swagger UI compatibility' });
 await expect(notes).toBeVisible();
 await page.locator('.topbar select').selectOption({ label: 'Reference' });
 await expect(page.locator('.info .title')).toContainText('Reference API');
 await expect(notes).toHaveCount(0);
 expect((await report(page)).diagnostics).toEqual([]);
 await page.locator('.topbar select').selectOption({ label: 'All endpoints' });
 await expect(page.locator('.info .title')).toContainText('Native methods API');
 await expect(notes).toHaveCount(1);
 expect((await report(page)).diagnostics).toHaveLength(5);
});
