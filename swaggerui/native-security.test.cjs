const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

// Exercise the shipped local resolver independently of Swagger UI's lossy OAuth form projection.
function resolve(document, name) {
 const window = {};
 vm.runInNewContext(fs.readFileSync(__dirname + '/native-security.js', 'utf8'), { window });
 return window.OpenAPINativeSecurity().fn.openapiSecurityScheme(document, name);
}

// Preserve exact field presence, escaped names, and native flow data without mutating the source.
test('native security metadata resolves exact local definitions', () => {
 const scheme = { type: 'oauth2', deprecated: false, oauth2MetadataUrl: '', flows: { deviceAuthorization: { tokenUrl: '/token' } } };
 const document = { openapi: '3.2.0', components: { securitySchemes: { 'Device/name~': scheme, Alias: { $ref: '#/components/securitySchemes/Device~1name~0' } } } };
 assert.equal(resolve(document, 'Alias'), scheme);
 assert.equal(resolve(document, 'Device/name~'), scheme);
 assert.equal(resolve(document, 'Absent'), undefined);
 assert.equal(resolve({ ...document, openapi: '3.1.0' }, 'Alias'), undefined);
 assert.equal(scheme.deprecated, false);
});

// Reject external, malformed, inherited, circular, and overlong references without network access.
test('security reference resolution is local and bounded', () => {
 const schemes = { Remote: { $ref: 'https://outside.invalid/security' }, Invalid: { $ref: '#/%XX' },
  Cycle: { $ref: '#/components/securitySchemes/Cycle' }, Inherited: { $ref: '#/components/securitySchemes/toString' } };
 for (let i = 0; i < 100; i++) schemes['s' + i] = { $ref: '#/components/securitySchemes/s' + (i + 1) };
 schemes.s100 = { type: 'http', scheme: 'bearer' };
 const document = { openapi: '3.2.0', components: { securitySchemes: schemes } };
 for (const name of ['Remote', 'Invalid', 'Cycle', 'Inherited', 's0', 'toString']) assert.equal(resolve(document, name), undefined);
 assert.equal(resolve(document, 's99'), schemes.s100);
});

// An unresolved native device definition must not restore the upstream grant button.
test('unresolved device panels stay read-only while ordinary OAuth controls survive', () => {
 const window = {};
 vm.runInNewContext(fs.readFileSync(__dirname + '/native-security.js', 'utf8'), { window });
 const Original = {};
 const system = { React: { createElement: (type, props, ...children) => ({ type, props, children }) },
  specSelectors: { specJson: () => ({ toJS: () => ({ openapi: '3.2.0' }) }) }, getComponent: () => ({}), authActions: {} };
 const render = window.OpenAPINativeSecurity().wrapComponents.oauth2(Original, system);
 const device = render({ name: 'Unresolved', schema: { get: key => key === 'flow' ? 'deviceAuthorization' : undefined } });
 assert.equal(device.type, 'section');
 assert.match(JSON.stringify(device), /Device authorization is not available/);
 assert.doesNotMatch(JSON.stringify(device), /Apply given OAuth2 credentials/);
 assert.equal(render({ name: 'Redirect', schema: { get: () => 'authorizationCode' } }).type, Original);
});
