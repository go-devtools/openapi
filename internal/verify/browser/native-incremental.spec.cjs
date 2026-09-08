const { test, expect } = require('./fixtures.cjs');
const { writeFile } = require('node:fs/promises');
const document = require('../testdata/browser/native-incremental.json');

// Separate receipt of real network chunks from the viewer's completed-response rendering.
test('native stream response display waits for completion despite flushed network chunks', async ({ page, request, context }, testInfo) => {
 await page.route('**/wire-enabled/docs/openapi.json', route => route.fulfill({ json: document }));
 await page.goto('/wire-enabled/docs/');
 const op = page.locator('.opblock-get');
 await expect(op.getByRole('button', { name: 'Execute', exact: true })).toBeVisible();
 const cdp = await context.newCDPSession(page);
 await cdp.send('Network.enable');
 await expect(op.getByLabel('Stream response display', { exact: true })).toContainText('only after the response completes');
 const ids = new Set();
 let bytes = 0;
 let finished = false;
 cdp.on('Network.responseReceived', event => { if (new URL(event.response.url).pathname === '/incremental-stream') ids.add(event.requestId); });
 cdp.on('Network.dataReceived', event => { if (ids.has(event.requestId)) bytes += event.dataLength; });
 page.on('requestfinished', req => { if (new URL(req.url()).pathname === '/incremental-stream') finished = true; });
 const evidence = [];
 try {
  for (const [media, expected] of [
   ['application/x-ndjson', '{"message":"flushed-first"}\n{"message":"released-second"}\n'],
   ['text/event-stream', 'event: first\ndata: flushed-first\n\nevent: second\ndata: released-second\n\n']
  ]) {
   await op.locator('tr.response[data-code="200"]').getByRole('combobox', { name: 'Media Type', exact: true }).selectOption(media);
   const clear = op.getByRole('button', { name: 'Clear', exact: true });
   if (await clear.count()) await clear.click();
   bytes = 0; finished = false; ids.clear();
   const responseReady = page.waitForResponse(response => new URL(response.url()).pathname === '/incremental-stream');
   await op.getByRole('button', { name: 'Execute', exact: true }).click();
   const response = await responseReady;
   await expect.poll(() => bytes).toBeGreaterThan(0);
   await page.evaluate(() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done))));
   expect(finished).toBe(false);
   await expect(op.locator('.live-responses-table')).toHaveCount(0);
   await expect(op).not.toContainText('flushed-first');
   evidence.push({ media, initialChunkBytes: bytes, responseFinishedBeforeRelease: finished, displayedBeforeRelease: false });
   const release = await request.post('/incremental-release');
   expect(release.status()).toBe(204);
   expect(await response.text()).toBe(expected);
   await expect(op.locator('.live-responses-table')).toContainText('released-second');
   await expect.poll(() => finished).toBe(true);
  }
  expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(document);
  const output = testInfo.outputPath('incremental-network.json');
  await writeFile(output, JSON.stringify(evidence, null, 2) + '\n');
  await testInfo.attach('incremental-network-evidence', { path: output, contentType: 'application/json' });
 } finally {
  await request.post('/incremental-release');
  await cdp.detach();
 }
});
