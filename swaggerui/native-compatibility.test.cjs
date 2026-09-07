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
 assert.equal(diagnostics.length, 4);
 assert.deepEqual(diagnostics.filter(d => d.code === 'openapi.ui.additionalOperations').map(d => d.route), ['NOTIFY {$request.body#/url}', 'PING changed']);
 assert.equal(diagnostics.filter(d => d.code === 'openapi.ui.reference').length, 2);
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
