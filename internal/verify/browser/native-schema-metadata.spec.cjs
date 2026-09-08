const { test, expect } = require('./fixtures.cjs');
const document = require('../testdata/browser/native-schema-metadata.json');

// Exercise metadata alongside compact examples through the actual upstream component tree.
async function schemaView(page, source = document) {
 await page.route('**/enabled/docs/openapi.json', route => route.fulfill({ json: source }));
 await page.goto('/enabled/docs/');
 await expect(page.locator('.info .title')).toContainText(document.info.title);
 const body = page.locator('.opblock-section-request-body');
 await body.getByRole('tab', { name: 'Schema', exact: true }).click();
 return body;
}

// Compact examples must retain sibling discriminator, XML, external docs and legacy example metadata.
test('schema examples retain upstream metadata and native default mappings', async ({ page }, testInfo) => {
 const body = await schemaView(page);
 const discriminator = body.locator('.json-schema-2020-12-keyword--discriminator');
 await expect(discriminator).toContainText('kind');
 await expect(discriminator).toContainText('Created');
 await expect(discriminator).not.toContainText('_opaque');
 await expect(body.getByRole('region', { name: 'Default mapping' })).toContainText('Unknown');
 await expect(body.getByRole('region', { name: 'Default mapping' })).toContainText('missing or unmatched');
 const xml = body.locator('.json-schema-2020-12-keyword--xml');
 await expect(xml).toContainText('change');
 await expect(xml).toContainText('urn:example:changes');
 await expect(body.getByLabel('XML node type', { exact: true })).toHaveText('element');
 await expect(body.getByRole('link', { name: 'https://example.invalid/schema-reference', exact: true })).toHaveAttribute('href','https://example.invalid/schema-reference');
 await expect(body).toContainText('legacy');
 const examples = body.getByRole('region', { name: 'Examples', exact: true });
 await expect(examples).toHaveCount(1);
 await expect(examples).toContainText('false');
 await expect(examples).not.toContainText('#0');
 await body.getByRole('button', { name: 'Change', exact: true }).click();
 await expect(body.getByRole('region', { name: 'Default mapping' })).toHaveCount(0);
 await body.getByRole('button', { name: 'Change', exact: true }).click();
 await expect(body.getByRole('region', { name: 'Default mapping' })).toContainText('Unknown');
 await page.screenshot({ path: testInfo.outputPath('schema-metadata-desktop.png'), fullPage: true });
 await page.setViewportSize({ width: 390, height: 844 });
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 await expect(body.getByRole('region', { name: 'Default mapping' })).toBeVisible();
 await page.screenshot({ path: testInfo.outputPath('schema-metadata-mobile.png'), fullPage: true });
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(document);
});

// The display adapter must also retain the original OpenAPI 3.1 schema metadata.
test('legacy schema metadata remains visible without native fields', async ({ page }) => {
 const source = structuredClone(document);
 source.openapi = '3.1.0';
 const schema = source.components.schemas.Change_opaque;
 delete schema.discriminator.defaultMapping;
 delete schema.xml.nodeType;
 schema.xml.attribute = true;
 delete schema.examples;
 const body = await schemaView(page, source);
 await expect(body.locator('.json-schema-2020-12-keyword--discriminator')).toContainText('kind');
 await expect(body.locator('.json-schema-2020-12-keyword--xml')).toContainText('attribute');
 await expect(body.getByRole('link', { name: 'https://example.invalid/schema-reference', exact: true })).toBeVisible();
 await expect(body).toContainText('legacy');
 await expect(body.getByRole('region', { name: 'Default mapping' })).toHaveCount(0);
 await expect(body.getByRole('region', { name: 'Examples', exact: true })).toHaveCount(0);
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(source);
});
