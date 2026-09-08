const { test, expect } = require('./fixtures.cjs');
const document = require('../testdata/browser/native-projection.json');

// Load native examples through the real shared viewer and an owned byte-echo endpoint.
async function operation(page, source = document) {
 await page.route('**/enabled/docs/openapi.json', route => route.fulfill({ json: source }));
 await page.goto('/enabled/docs/');
 await expect(page.locator('.info .title')).toContainText(document.info.title);
 const op = page.locator('.opblock-post');
 await expect(op.locator('tr[data-param-name]')).toHaveCount(2);
 return op;
}

// Parameter data values must be encoded exactly once and preserve numeric zero.
test('native parameter examples populate editable inputs and actual request values', async ({ page }) => {
 const op = await operation(page);
 await expect(op.locator('tr[data-param-name="q"] input[type="text"]')).toHaveValue('star field');
 await expect(op.locator('tr[data-param-name="X-Number"] input[type="text"]')).toHaveValue('0');
 await op.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/json');
 const received = page.waitForResponse(r => new URL(r.url()).pathname === '/native-submit');
 await op.getByRole('button', { name: 'Execute', exact: true }).click();
 const sent = (await received).request();
 expect(new URL(sent.url()).search).toBe('?q=star%20field');
 expect(sent.headers()['x-number']).toBe('0');
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(document);
});

// A default native example is not a manual edit and must not leak into a different media type.
test('media changes replace native defaults while preserving actual user edits', async ({ page }) => {
 const op = await operation(page);
 const body = op.locator('.opblock-section-request-body');
 await expect(body.locator('textarea')).toHaveValue('[\n  {\n    "name": "stars"\n  },\n  "payload"\n]');
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/json');
 await expect(body.locator('textarea')).toHaveValue('{\n  "name": "regular"\n}');
 await body.locator('textarea').fill('{"name":"edited"}');
 await op.getByRole('combobox', { name: 'Media Type', exact: true }).selectOption('application/json');
 await expect(body.locator('textarea')).toHaveValue('{"name":"edited"}');
});

// Form controls retain logical values and selections rather than replacing false with sampled true.
test('native form examples fill fields and follow explicit example selections', async ({ page }, testInfo) => {
 const op = await operation(page);
 const body = op.locator('.opblock-section-request-body');
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/x-www-form-urlencoded');
 const label = body.locator('tr[data-property-name="label"] input[type="text"]');
 const active = body.locator('tr[data-property-name="active"] select');
 await expect(label).toHaveValue('star field');
 await expect(active).toHaveValue('false');
 for (const [name, value, bool] of [['second','second field','true'], ['form','star field','false']]) {
  await body.getByRole('combobox', { name: 'Form example', exact: true }).selectOption(name);
  await expect(label).toHaveValue(value);
  await expect(active).toHaveValue(bool);
  const received = page.waitForResponse(r => new URL(r.url()).pathname === '/native-submit');
  await op.getByRole('button', { name: 'Execute', exact: true }).click();
  const result = await (await received).json();
  expect(result.contentType).toBe('application/x-www-form-urlencoded');
  expect(Object.fromEntries(new URLSearchParams(result.body))).toEqual({label: value, active: bool});
  expect(result.body).not.toContain('%2520');
 }
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/json');
 await expect(body.locator('textarea')).toHaveValue('{\n  "name": "regular"\n}');
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/x-www-form-urlencoded');
 await expect(label).toHaveValue('star field');
 await label.fill('manual field');
 await expect(label).toHaveValue('manual field');
 await page.screenshot({ path: testInfo.outputPath('native-form-desktop.png'), fullPage: true });
 await page.setViewportSize({ width: 390, height: 844 });
 await expect(label).toHaveValue('manual field');
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 await page.screenshot({ path: testInfo.outputPath('native-form-mobile.png'), fullPage: true });
});

// Unsupported positional multipart encoding must not submit JSON under a multipart Content-Type.
test('native body encoding limits affect only the selected request media', async ({ page }, testInfo) => {
 const sent = [];
 page.on('request', r => { if (new URL(r.url()).pathname === '/native-submit') sent.push(r.url()); });
 const op = await operation(page);
 const body = op.locator('.opblock-section-request-body');
 await expect(body.getByRole('region', { name: 'Request encoding limitation' })).toContainText('prefixEncoding');
 await expect(op.getByRole('button', { name: 'Execute', exact: true })).toBeDisabled();
 const blocked = await page.evaluate(() => window.ui.specActions.executeRequest({ pathName: '/native-submit', method: 'post' }));
 expect(blocked).toMatchObject({ type: 'openapi.ui.request.blocked', payload: { code: 'openapi.ui.multipart' } });
 expect(sent).toEqual([]);
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/xml');
 await expect(body.getByRole('region', { name: 'Request encoding limitation' })).toContainText('nodeType');
 await expect(op.getByRole('button', { name: 'Execute', exact: true })).toBeDisabled();
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/json');
 await expect(op.getByRole('button', { name: 'Execute', exact: true })).toBeEnabled();
 await expect(body.getByRole('region', { name: 'Request encoding limitation' })).toHaveCount(0);
 await expect(body.locator('textarea')).toHaveValue('{\n  "name": "regular"\n}');
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('multipart/mixed');
 await expect(op.getByRole('button', { name: 'Execute', exact: true })).toBeDisabled();
 await page.screenshot({ path: testInfo.outputPath('native-encoding-desktop.png'), fullPage: true });
 await page.setViewportSize({ width: 390, height: 844 });
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 await page.screenshot({ path: testInfo.outputPath('native-encoding-mobile.png'), fullPage: true });
});

// Explicit native XML wire examples bypass the unsupported sampler and remain submit-ready.
test('serialized XML examples preserve attributes and CDATA without generated substitutions', async ({ page }) => {
 const source = structuredClone(document);
 const bytes = '<message id="7"><![CDATA[hello & stars]]></message>';
 source.paths['/native-submit'].post.requestBody.content['application/xml'].examples = { exact: { serializedValue: bytes } };
 const op = await operation(page, source);
 const body = op.locator('.opblock-section-request-body');
 await body.getByRole('combobox', { name: 'Request content type', exact: true }).selectOption('application/xml');
 await expect(body.locator('textarea')).toHaveValue(bytes);
 await expect(op.getByRole('button', { name: 'Execute', exact: true })).toBeEnabled();
 const received = page.waitForResponse(r => new URL(r.url()).pathname === '/native-submit');
 await op.getByRole('button', { name: 'Execute', exact: true }).click();
 expect(await (await received).json()).toEqual({ body: bytes, contentType: 'application/xml' });
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(source);
});

// Header examples need their own display because the upstream table only renders Schema.example.
test('response header native examples show logical and serialized values', async ({ page }) => {
 const op = await operation(page);
 const panel = op.getByRole('region', { name: 'Response header examples' });
 await expect(panel).toContainText('X-Reply');
 await panel.getByText('X-Reply — native', { exact: true }).click();
 await expect(panel.getByLabel('Header data value', { exact: true })).toHaveText('0');
 await expect(panel.getByLabel('Header serialized value', { exact: true })).toHaveText('0');
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(document);
});
