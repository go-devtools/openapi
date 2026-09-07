const { test, expect } = require('./fixtures.cjs');

// Verify public rendering and actual bytes; schema sampling must not replace explicit native values.
const jsonCases = [
 ['logical', '{\n  "Role": "editor",\n  "Count": 0,\n  "Enabled": false,\n  "Note": null\n}'],
 ['serialized', '{ "amount":9007199254740993, "label":"wire" }'],
 ['paired', '{"Role":"editor","Count":0}'],
 ['zero', '0'], ['false', 'false'], ['null', 'null'],
 ['empty string', '""'], ['empty array', '[]'], ['empty object', '{}'],
 ['legacy', '{\n  "Role": "admin"\n}'],
 ['business fields', '{\n  "serializedValue": "literal property",\n  "dataValue": {\n    "value": false\n  }\n}'],
 ['JSON-looking string', '"{\\"text\\":\\"still a string\\"}"']
];

// Read selectors through their rendered controls rather than private Redux state.
async function nativeOperation(page, enabled = false) {
 await page.goto(enabled ? '/native-enabled/docs/' : '/native/docs/');
 await expect(page.locator('.info .title')).toContainText('Native examples API');
 return page.locator('.opblock').filter({ hasText: '/native-submit' });
}

// A native response example must survive selection, falsy values, and reference resolution.
test('native response examples display exact logical and serialized values', async ({ page, request }) => {
 const operation = await nativeOperation(page);
 const response = operation.locator('tr.response[data-code="200"]');
 const choices = response.locator('.response-control-examples select');
 const code = response.locator('.highlight-code .microlight').first();
 for (const [name, expected] of jsonCases) {
  await choices.selectOption(name);
  await expect.poll(() => code.textContent()).toBe(expected);
 }
 await expect(operation.getByRole('button', { name: 'Execute', exact: true })).toHaveCount(0);
 const original = await (await request.get('/native/docs/openapi.json')).json();
 expect(original.openapi).toBe('3.2.0');
 expect(original.components.examples.Logical).not.toHaveProperty('value');
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(original);
 expect(original.paths['/native-submit'].post.responses['200'].content['application/json'].examples.serialized.serializedValue)
  .toBe(jsonCases[1][1]);
});

// Exact wire examples may contain XML, empty strings, or SSE framing without HTML execution.
test('native response examples preserve non-JSON wire text and media references', async ({ page }) => {
 const operation = await nativeOperation(page);
 const response = operation.locator('tr.response[data-code="200"]');
 const cases = [
  ['application/xml', 'XML', '<user role="editor">A &amp; B</user>'],
  ['text/plain', 'text', '<img src=x onerror=alert(1)> & stars'],
  ['text/plain', 'logical text', 'Stars & spaces'],
  ['application/problem+json; charset=utf-8', 'problem', '{\n  "status": 0,\n  "detail": ""\n}'],
  ['text/plain', 'empty', ''],
  ['text/event-stream', 'event', 'event: updated\ndata: {"count":0}\n\n']
 ];
 for (const [media, name, expected] of cases) {
  await response.getByRole('combobox', { name: 'Media Type', exact: true }).selectOption(media);
  await response.locator('.response-control-examples select').selectOption(name);
  const code = response.locator('.highlight-code .microlight').first();
  await expect(code).toHaveCount(1);
  await expect.poll(() => code.textContent()).toBe(expected);
 }
 await expect(response.locator('img')).toHaveCount(0);
});

// Preserve literal wire formats when submitting through media changes or returning to an edited value.
test('native request wire examples survive media changes and manual editing', async ({ page }, testInfo) => {
 const operation = await nativeOperation(page, true);
 const requestBody = operation.locator('.opblock-section-request-body');
 const media = requestBody.getByRole('combobox', { name: 'Request content type', exact: true });
 for (const [type, name, expected] of [
  ['application/xml', 'XML', '<user role="editor">A &amp; B</user>'],
  ['text/plain', 'text', '<img src=x onerror=alert(1)> & stars'],
  ['text/plain', 'logical text', 'Stars & spaces'],
  ['application/problem+json; charset=utf-8', 'problem', '{\n  "status": 0,\n  "detail": ""\n}'],
  ['text/plain', 'empty', '']
 ]) {
  await media.selectOption(type);
  await requestBody.locator('.examples-select select').selectOption(name);
  await expect(requestBody.locator('textarea')).toHaveValue(expected);
  const received = page.waitForResponse(response => response.url().endsWith('/native-submit') && response.request().method() === 'POST');
  await operation.getByRole('button', { name: 'Execute', exact: true }).click();
  expect(await (await received).json()).toEqual({ body: expected, contentType: type });
 }
 await media.selectOption('application/json');
 await requestBody.locator('.examples-select select').selectOption('logical');
 const edited = '{"Role":"manually edited"}';
 await requestBody.locator('textarea').fill(edited);
 await operation.locator('tr.response[data-code="200"] .response-control-examples select').selectOption('serialized');
 await expect(requestBody.locator('textarea')).toHaveValue(edited);
 await requestBody.locator('.examples-select select').selectOption('paired');
 await expect(requestBody.locator('textarea')).toHaveValue(jsonCases[2][1]);
 await page.screenshot({ path: testInfo.outputPath('native-examples.png'), fullPage: true });
});

// Show the schema-ready value separately when authors also supply its explicit wire serialization.
test('paired native examples retain their logical data alongside wire text', async ({ page }) => {
 const operation = await nativeOperation(page);
 const response = operation.locator('tr.response[data-code="200"]');
 await response.locator('.response-control-examples select').selectOption('paired');
 await expect(response.getByLabel('Data value', { exact: true })).toContainText('"Role": "editor"');
 await expect(response.getByLabel('Data value', { exact: true })).toContainText('"Count": 0');
 await response.locator('.response-control-examples select').selectOption('serialized');
 await expect(response.getByLabel('Data value', { exact: true })).toHaveCount(0);
});

// Submission must use the selected native value, including JSON primitives and unrounded wire numbers.
test('native request examples submit the selected JSON bytes', async ({ page }) => {
 const operation = await nativeOperation(page, true);
 const requestBody = operation.locator('.opblock-section-request-body');
 for (const [name, expected] of jsonCases) {
  await requestBody.locator('.examples-select select').selectOption(name);
  await expect(requestBody.locator('textarea')).toHaveValue(expected);
  const received = page.waitForResponse(response => response.url().endsWith('/native-submit') && response.request().method() === 'POST');
  await operation.getByRole('button', { name: 'Execute', exact: true }).click();
  const response = await received;
  expect(response.status()).toBe(200);
  expect(await response.json()).toEqual({ body: expected, contentType: 'application/json' });
 }
});
