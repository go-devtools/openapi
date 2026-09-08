const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

// Load the actual offline plugin so capability tests exercise the shipped scanner.
function plugin() {
 const window = {};
 vm.runInNewContext(fs.readFileSync(__dirname + '/native-compatibility.js', 'utf8'), { window });
 return window.OpenAPINativeCompatibility();
}

// Normalize cross-realm records for assertions without modifying the supplied specification.
function inspect(document) { return JSON.parse(JSON.stringify(plugin().fn.openapiUICompatibility(document))); }

// Follow valid local references while preserving custom method case and escaped source locations.
test('native gaps include referenced paths and preserve operation identity', () => {
 const document = { openapi: '3.2.0', paths: { '/items': { $ref: '#/paths/~1source~0path' }, '/source~path': { additionalOperations: { SEARCH: { summary: 'Upper' }, search: { summary: 'Lower' } } } } };
 const before = JSON.stringify(document);
 const diagnostics = inspect(document).diagnostics;
 assert.deepEqual(diagnostics.map(d => d.route), ['SEARCH /items', 'search /items', 'SEARCH /source~path', 'search /source~path']);
 assert.match(diagnostics[0].message, /^#\/paths\/~1source~0path\/additionalOperations\/SEARCH:/);
 assert.equal(JSON.stringify(document), before);
});

// Treat metadata-like application data as opaque and keep previous specifications unaffected.
test('examples, schemas, extensions and earlier OpenAPI versions do not create false diagnostics', () => {
 const data = { additionalOperations: { DELETE: {} }, tags: [{ name: 'Fake', parent: 'Fake' }] };
 const document = { openapi: '3.2.0', paths: { '/items': { post: { requestBody: { content: { 'application/json': { example: data } } }, 'x-data': data } }, 'x-data': data },
  components: { schemas: { Data: { properties: data } } }, 'x-data': data };
 assert.deepEqual(inspect(document).diagnostics, []);
 assert.deepEqual(inspect({ openapi: '3.1.0', tags: [{ name: 'Earlier', parent: 'Parent' }] }).diagnostics, []);
});

// Inspect callback and webhook Path Items without fetching unresolved resources or following cycles indefinitely.
test('callbacks, webhooks and reference cycles retain bounded diagnostics', () => {
 const document = { openapi: '3.2.0', paths: { '/items': { $ref: '#/paths/~1items', get: { callbacks: { local: { '{$request.body#/url}': { additionalOperations: { NOTIFY: {} } } },
  remote: { $ref: 'https://outside.invalid/callback.json' } } } } }, webhooks: { changed: { additionalOperations: { PING: {} } }, remote: { $ref: 'https://outside.invalid/path.json' } } };
 const diagnostics = inspect(document).diagnostics;
 assert.equal(diagnostics.length, 6);
 assert.deepEqual(diagnostics.filter(d => d.code === 'openapi.ui.additionalOperations').map(d => d.route), ['NOTIFY {$request.body#/url}', 'PING changed']);
 assert.equal(diagnostics.filter(d => d.code === 'openapi.ui.reference').length, 2);
 assert.equal(diagnostics.filter(d => d.code === 'openapi.ui.webhooks').length, 2);
});

// Distinguish empty-but-present tag metadata from absent fields and report both work and output limits.
test('tag presence and compatibility budgets cannot imply full coverage', () => {
 const diagnostics = inspect({ openapi: '3.2.0', tags: [{ name: '', summary: '', parent: '', kind: '' }] }).diagnostics;
 assert.match(diagnostics[0].message, /summary: "", parent: "", kind: ""/);
 const tags = Array.from({ length: 300 }, (_, i) => ({ name: String(i), summary: 'Label' }));
 const report = inspect({ openapi: '3.2.0', tags });
 assert.equal(report.diagnostics.length, 201);
 assert.equal(report.diagnostics.at(-1).code, 'openapi.ui.inspect.limit');
 const document = { openapi: '3.2.0', paths: { '/deep': { $ref: '#/components/pathItems/p0' } }, components: { pathItems: {} } };
 for (let i = 0; i < 100; i++) document.components.pathItems['p' + i] = { $ref: '#/components/pathItems/p' + (i + 1) };
 assert.equal(inspect(document).diagnostics.at(-1).code, 'openapi.ui.inspect.limit');
});

// Preserve actual route identities and the source pointer of inherited whole-query parameters.
test('whole-query diagnostics keep inherited references and custom method case', () => {
 const document = { openapi: '3.2.0', paths: { '/items': { parameters: [{ $ref: '#/components/parameters/Whole' }],
  get: {}, additionalOperations: { search: {} } } }, components: { parameters: { Whole: { in: 'querystring', name: 'all' } } } };
 const diagnostics = inspect(document).diagnostics.filter(d => d.code === 'openapi.ui.querystring');
 assert.deepEqual(diagnostics.map(d => d.route), ['GET /items', 'search /items']);
 assert.ok(diagnostics.every(d => d.message.startsWith('#/components/parameters/Whole:')));
});

// Identify device exchange and discovery limits without treating security-like example data as declarations.
test('native security diagnostics follow local references and leave payloads opaque', () => {
 const device = { type: 'oauth2', deprecated: true, oauth2MetadataUrl: 'https://oauth.invalid/metadata', flows: { deviceAuthorization: {} } };
 const document = { openapi: '3.2.0', components: { securitySchemes: { 'Device/name~': device, Alias: { $ref: '#/components/securitySchemes/Device~1name~0' },
  Bearer: { type: 'http', scheme: 'bearer', deprecated: false }, Cycle: { $ref: '#/components/securitySchemes/Cycle' }, Remote: { $ref: 'https://outside.invalid/security' } },
  examples: { Fake: { value: { components: { securitySchemes: { device } } } } } }, 'x-security': device };
 const before = JSON.stringify(document);
 const diagnostics = inspect(document).diagnostics;
 assert.deepEqual(diagnostics.map(d => d.code), ['openapi.ui.deviceAuthorization', 'openapi.ui.oauth2Metadata', 'openapi.ui.deviceAuthorization', 'openapi.ui.oauth2Metadata', 'openapi.ui.reference']);
 assert.ok(diagnostics.slice(0, 4).every(d => d.message.startsWith('#/components/securitySchemes/Device~1name~0/')));
 assert.equal(JSON.stringify(document), before);
 assert.deepEqual(inspect({ ...document, openapi: '3.1.0' }).diagnostics, []);
});

// Stop deeply referenced and oversized security collections within the shared inspection budget.
test('security inspection cannot silently exceed work or diagnostic budgets', () => {
 const schemes = {};
 for (let i = 0; i < 100; i++) schemes['s' + i] = { $ref: '#/components/securitySchemes/s' + (i + 1) };
 assert.equal(inspect({ openapi: '3.2.0', components: { securitySchemes: schemes } }).diagnostics.at(-1).code, 'openapi.ui.inspect.limit');
 for (let i = 0; i < 300; i++) schemes['s' + i] = { type: 'oauth2', flows: { deviceAuthorization: {} } };
 assert.equal(inspect({ openapi: '3.2.0', components: { securitySchemes: schemes } }).diagnostics.length, 201);
});

// Keep webhook locations stable without scanning application payloads or exceeding report limits.
test('webhook display limits preserve names, references and bounded output', () => {
 const document = { openapi: '3.2.0', webhooks: { 'Event/~': { $ref: '#/components/pathItems/Event' } },
  paths: { '/normal': { get: { responses: { '200': { description: 'OK' } } } } },
  components: { pathItems: { Event: { post: { responses: { '204': { description: 'Accepted' } } } } },
   examples: { Fake: { value: { webhooks: { Fake: {} } } } } } };
 const before = JSON.stringify(document);
 const diagnostics = inspect(document).diagnostics;
 assert.equal(diagnostics.length, 1);
 assert.equal(diagnostics[0].code, 'openapi.ui.webhooks');
 assert.match(diagnostics[0].message, /^#\/webhooks\/Event~1~0:/);
 assert.equal(JSON.stringify(document), before);
 const webhooks = Object.fromEntries(Array.from({ length: 300 }, (_, index) => ['event' + index, {}]));
 const bounded = inspect({ openapi: '3.2.0', webhooks }).diagnostics;
 assert.equal(bounded.length, 201);
 assert.equal(bounded.at(-1).code, 'openapi.ui.inspect.limit');
});
