const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

// Exercise the shipped native wire plugin without a network client or application server.
function plugin() {
 const window = {};
 vm.runInNewContext(fs.readFileSync(__dirname + '/native-wire.js', 'utf8'), { window });
 return window.OpenAPINativeWire();
}

// Resolve inherited and aliased parameter contracts without looking inside application payloads.
test('whole-query detection follows local aliases and remains operation-specific', () => {
 const document = { openapi: '3.2.0', paths: {
  '/direct': { get: { parameters: [{ in: 'querystring', name: '' }] }, post: { parameters: [{ in: 'query', name: 'normal' }] } },
  '/inherited': { parameters: [{ $ref: '#/components/parameters/Query' }], get: {} },
  '/linked': { $ref: '#/paths/~1inherited' },
  '/ordinary': { get: { requestBody: { content: { 'application/json': { example: { parameters: [{ in: 'querystring' }] } } } } } }
 }, components: { parameters: { Query: { $ref: '#/components/parameters/Alias' }, Alias: { in: 'querystring', name: 'query' } } } };
 const has = plugin().fn.openapiHasWholeQuery;
 assert.equal(has(document, '/direct', 'get'), true);
 assert.equal(has(document, '/direct', 'post'), false);
 assert.equal(has(document, '/inherited', 'get'), true);
 assert.equal(has(document, '/linked', 'get'), true);
 assert.equal(has(document, '/ordinary', 'get'), false);
 assert.equal(has({ ...document, openapi: '3.1.0' }, '/direct', 'get'), false);
 assert.equal(has({ openapi: '3.2.0' }, '/callback', 'post', { parameters: [{ in: 'querystring', name: 'body-query' }] }), true);
});

// Unknown or cyclic local references are bounded and do not cause network retrieval or prototype lookups.
test('whole-query detection bounds unresolved local reference traversal', () => {
 const has = plugin().fn.openapiHasWholeQuery;
 const document = { openapi: '3.2.0', paths: { '/cycle': { $ref: '#/paths/~1cycle' }, '/remote': { $ref: 'https://outside.invalid/path' },
  '/prototype': { $ref: '#/__proto__/query' } } };
 for (const path of Object.keys(document.paths)) assert.equal(has(document, path, 'get'), false);
});

// Return a structured no-request result while preserving ordinary requests and refreshing cached specifications.
test('request guard avoids lossy execution and invalidates its document cache', () => {
 const feature = plugin();
 let calls = 0;
 let copies = 0;
 let document = { openapi: '3.2.0', paths: { '/items': { get: { parameters: [{ in: 'querystring', name: 'all' }] } } } };
 let source = { toJS: () => { copies++; return document; } };
 const system = { specSelectors: { specJson: () => source } };
 const execute = feature.statePlugins.spec.wrapActions.executeRequest(request => { calls++; return request; }, system);
 const request = { pathName: '/items', method: 'get' };
 const before = JSON.stringify(document);
 assert.equal(execute(request).payload.code, 'openapi.ui.querystring');
 assert.equal(execute(request).type, 'openapi.ui.request.blocked');
 assert.equal(calls, 0);
 assert.equal(copies, 1);
 assert.equal(JSON.stringify(document), before);
 document = { openapi: '3.2.0', paths: { '/items': { get: {} } } };
 source = { toJS: () => { copies++; return document; } };
 assert.equal(execute(request), request);
 assert.equal(calls, 1);
 assert.equal(copies, 2);
});
