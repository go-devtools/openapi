const { test, expect } = require('./fixtures.cjs');

// Keep known lossy whole-query serialization unavailable even when ordinary GET submission is enabled.
test('whole-query parameters retain read-only details and reject lossy submission', async ({ page, request }) => {
 const sent = [];
 page.on('request', request => { if (new URL(request.url()).pathname === '/querystring-submit') sent.push(request.url()); });
 await page.goto('/wire-enabled/docs/');
 const operation = page.locator('.opblock').filter({ hasText: '/querystring-submit' });
 await expect(operation.locator('tr.response[data-code="200"]')).toBeVisible();
 await expect(operation).toContainText('(querystring)');
 await expect(operation.locator('.openapi-wire-note')).toContainText('does not serialize whole-query parameters');
 await expect(operation.locator('textarea')).not.toBeEditable();
 await expect(operation.getByRole('button', { name: 'Execute', exact: true })).toHaveCount(0);
 await expect(operation.getByRole('button', { name: 'Try it out', exact: true })).toHaveCount(0);
 const notes = page.getByRole('region', { name: 'Swagger UI compatibility' });
 await notes.locator('summary').click();
 await expect(notes).toContainText('openapi.ui.querystring');
 await expect(notes).toContainText('#/paths/~1querystring-submit/get/parameters/0:');
 const blocked = await page.evaluate(() => window.ui.specActions.executeRequest({
  pathName: '/querystring-submit', method: 'get', operation: window.ui.specSelectors.operationWithMeta('/querystring-submit', 'get')
 }));
 expect(blocked).toMatchObject({ type: 'openapi.ui.request.blocked', payload: { code: 'openapi.ui.querystring', route: 'GET /querystring-submit' } });
 expect(sent).toEqual([]);
 const original = await (await request.get('/wire-enabled/docs/openapi.json')).json();
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(original);
 await expect(page.locator('.opblock').filter({ hasText: '/native-stream' }).getByRole('button', { name: 'Execute', exact: true })).toBeVisible();
});

// Present per-item schemas separately from whole-body examples, including reused media and boolean schemas.
test('stream item schemas follow response media and preserve native semantics', async ({ page, request }, testInfo) => {
 await page.goto('/wire/docs/');
 const operation = page.locator('.opblock').filter({ hasText: '/native-stream' });
 const response = operation.locator('tr.response[data-code="200"]');
 const panel = operation.getByRole('region', { name: 'Response 200 stream item schema' });
 await expect(panel).toBeVisible();
 await expect(panel).toContainText('application/x-ndjson');
 await expect(panel).toContainText('message');
 await expect(panel).toContainText('string');
 await expect(panel).toContainText('One record in the stream.');
 const media = response.getByRole('combobox', { name: 'Media Type', exact: true });
 await media.selectOption('text/event-stream');
 await expect(panel).toContainText('text/event-stream');
 await expect(panel).toContainText('data');
 await expect(panel).toContainText('string');
 await expect(response.locator('.highlight-code .microlight').first()).toHaveText('event: update\ndata: {"message":"first"}\n\n');
 await media.selectOption('application/jsonl');
 await expect(panel).toContainText('false — No stream item is valid.');
 await media.selectOption('application/json-seq');
 await expect(panel).toContainText('true — Any stream item is allowed.');
 await media.selectOption('application/x-ndjson');
 await expect(panel).toContainText('message');
 const body = page.locator('.opblock').filter({ hasText: '/stream-submit' });
 await expect(body.getByRole('region', { name: 'Request stream item schema' })).toContainText('message');
 await expect(body.getByRole('button', { name: 'Execute', exact: true })).toHaveCount(0);
 const original = await (await request.get('/wire/docs/openapi.json')).json();
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(original);
 expect(original.paths['/stream-submit'].post.requestBody.content['application/x-ndjson'].schema).toMatchObject({ type: 'array', maxItems: 2 });
 await page.screenshot({ path: testInfo.outputPath('native-stream-schemas.png'), fullPage: true });
 await page.setViewportSize({ width: 390, height: 844 });
 await expect(panel).toBeVisible();
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

// Preserve exact finite stream bytes in both directions; this does not claim an incremental browser stream reader.
test('item schema display preserves NDJSON submission and NDJSON or SSE response framing', async ({ page }) => {
 await page.goto('/wire-enabled/docs/');
 const upload = page.locator('.opblock').filter({ hasText: '/stream-submit' });
 const bytes = '{"message":"first"}\n{"message":"second"}\n';
 await expect(upload.locator('textarea')).toHaveValue(bytes);
 await expect(upload.getByRole('region', { name: 'Request stream item schema' })).toContainText('message');
 const received = page.waitForResponse(response => new URL(response.url()).pathname === '/stream-submit');
 await upload.getByRole('button', { name: 'Execute', exact: true }).click();
 expect(await (await received).json()).toEqual({ body: bytes, contentType: 'application/x-ndjson' });
 const download = page.locator('.opblock').filter({ hasText: '/native-stream' });
 const response = download.locator('tr.response[data-code="200"]');
 for (const [media, expected] of [['application/x-ndjson', bytes], ['text/event-stream', 'event: update\ndata: {"message":"first"}\n\n']]) {
  await response.getByRole('combobox', { name: 'Media Type', exact: true }).selectOption(media);
  const received = page.waitForResponse(response => new URL(response.url()).pathname === '/native-stream');
  await download.getByRole('button', { name: 'Execute', exact: true }).click();
  const result = await received;
  expect(result.status()).toBe(200);
  expect(result.headers()['content-type']).toBe(media);
  expect(await result.text()).toBe(expected);
 }
});
