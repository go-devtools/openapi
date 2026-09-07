const { test, expect } = require('./fixtures.cjs');

// Keep device endpoints readable without presenting an unsupported grant action or issuing discovery requests.
test('device authorization shows native fields and explicit limits on desktop and mobile', async ({ page, request }, testInfo) => {
 await page.goto('/security/docs/');
 await expect(page).toHaveTitle('Browser contract');
 await expect(page.locator('.info .title')).toContainText('Native security API');
 const original = await (await request.get('/security/docs/openapi.json')).json();
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().toJS())).toEqual(original);
 const diagnostics = await page.evaluate(() => window.ui.fn.openapiUICompatibility(window.ui.specSelectors.specJson().toJS()).diagnostics);
 expect(diagnostics.map(d => d.code)).toEqual(['openapi.ui.deviceAuthorization', 'openapi.ui.oauth2Metadata', 'openapi.ui.oauth2Metadata']);
 await page.locator('.auth-wrapper').getByRole('button', { name: 'Authorize' }).click();
 const dialog = page.locator('.dialog-ux');
 const device = dialog.getByRole('region', { name: 'Device device authorization', exact: true });
 await expect(device).toBeVisible();
 await expect(device).toContainText('Deprecated security scheme');
 await expect(device).toContainText('Device authorization URL: https://oauth.invalid/device');
 await expect(device).toContainText('Token URL: https://oauth.invalid/token');
 await expect(device).toContainText('Refresh URL: https://oauth.invalid/refresh');
 await expect(device).toContainText('OAuth metadata URL: https://oauth.invalid/metadata');
 await expect(device).toContainText('Read a sample <img src=x onerror=alert(1)>');
 await expect(device.locator('img')).toHaveCount(0);
 await expect(device).toContainText('Device authorization is not available in this viewer.');
 await expect(device.getByRole('button', { name: /Authorize|Apply/ })).toHaveCount(0);
 await expect(device.locator('input, a')).toHaveCount(0);
 const redirect = dialog.getByLabel('Redirect security scheme', { exact: true });
 await expect(redirect).toContainText('OAuth metadata URL: https://oauth.invalid/redirect-metadata');
 await expect(redirect).not.toContainText('Deprecated security scheme');
 await expect(redirect.getByRole('button', { name: 'Apply given OAuth2 credentials', exact: true })).toBeVisible();
 await expect(redirect.locator('input[id="client_id_authorizationCode"]')).toHaveCount(1);
 const bearer = dialog.getByLabel('BearerAuth security scheme', { exact: true });
 await expect(bearer).toContainText('Deprecated security scheme');
 await expect(bearer.locator('input')).toHaveCount(1);
 await device.scrollIntoViewIfNeeded();
 await page.screenshot({ path: testInfo.outputPath('native-security-desktop.png') });
 await page.setViewportSize({ width: 390, height: 844 });
 await device.scrollIntoViewIfNeeded();
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
 expect(await device.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
 await page.screenshot({ path: testInfo.outputPath('native-security-mobile.png') });
 await device.getByRole('button', { name: 'Close', exact: true }).click();
 await expect(dialog).toHaveCount(0);
 await page.locator('.topbar select').selectOption({ label: 'Reference' });
 await expect(page.locator('.info .title')).toContainText('Reference API');
 await expect(page.getByRole('region', { name: 'Swagger UI compatibility' })).toHaveCount(0);
 await page.locator('.auth-wrapper').getByRole('button', { name: 'Authorize' }).click();
 await expect(page.locator('.dialog-ux')).not.toContainText('Deprecated security scheme');
 await expect(page.locator('.dialog-ux')).not.toContainText('deviceAuthorization');
});

// Native deprecation metadata is informational and must not change a working explicit Bearer submission.
test('native security metadata preserves Bearer authorization and source bytes', async ({ page }) => {
 await page.goto('/security-enabled/docs/');
 await page.locator('.auth-wrapper').getByRole('button', { name: 'Authorize' }).click();
 const dialog = page.locator('.dialog-ux');
 await dialog.getByLabel('BearerAuth security scheme', { exact: true }).locator('input').fill('native-demo-token');
 await dialog.getByRole('button', { name: 'Apply credentials', exact: true }).click();
 await dialog.getByRole('button', { name: 'Close', exact: true }).first().click();
 const operation = page.locator('.opblock-post');
 await operation.locator('textarea').fill('{"Role":"admin"}');
 const responsePromise = page.waitForResponse(response => response.url().endsWith('/submit') && response.request().method() === 'POST');
 await operation.getByRole('button', { name: 'Execute', exact: true }).click();
 const response = await responsePromise;
 expect(response.status()).toBe(200);
 expect(response.request().headers().authorization).toBe('Bearer native-demo-token');
 expect(await response.json()).toEqual({ Role: 'admin' });
 expect(await page.evaluate(() => window.ui.specSelectors.specJson().getIn(['components', 'securitySchemes', 'BearerAuth', 'deprecated']))).toBe(true);
});
