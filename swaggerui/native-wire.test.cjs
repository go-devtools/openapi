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

// Keep positional encoding decisions scoped to a selected media type and resolve only local aliases.
test('native request body limits retain supported alternatives and exact XML examples', () => {
 const body = { content: {
  'multipart/mixed': { prefixEncoding: [{ contentType: 'application/json' }], itemEncoding: { contentType: 'text/plain' } },
  'multipart/form-data': { schema: { type: 'object', properties: { name: { type: 'string' } } } },
  'application/json': { example: { xml: { nodeType: 'cdata' } } },
  'application/xml': { schema: { $ref: '#/components/schemas/XML' }, examples: { logical: { dataValue: { text: 'value' } }, wire: { $ref: '#/components/examples/XML' } } }
 } };
 const document = { openapi: '3.2.0', paths: { '/body': { post: { requestBody: { $ref: '#/components/requestBodies/Body' } } } }, components: {
  requestBodies: { Body: body }, schemas: { XML: { type: 'object', properties: { text: { type: 'string', xml: { nodeType: 'cdata' } } } } },
  examples: { XML: { serializedValue: '<message><![CDATA[value]]></message>' } }
 } };
 const inspect = plugin().fn.openapiBodyLimitation;
 const before = JSON.stringify(document);
 assert.equal(inspect(document, '/body', 'post', 'multipart/mixed').code, 'openapi.ui.multipart');
 assert.equal(inspect(document, '/body', 'post', 'multipart/form-data'), null);
 assert.equal(inspect(document, '/body', 'post', 'application/json'), null);
 assert.equal(inspect(document, '/body', 'post', 'application/xml', 'logical').code, 'openapi.ui.xmlNodeType');
 assert.equal(inspect(document, '/body', 'post', 'application/xml', 'wire'), null);
 assert.equal(inspect({ ...document, openapi: '3.1.0' }, '/body', 'post', 'multipart/mixed'), null);
 assert.equal(JSON.stringify(document), before);
});

// The action guard must use the current selection and emit no request for a lossy body.
test('native request body action guard follows media selection and refreshed documents', () => {
 const feature = plugin();
 let calls = 0;
 let selected = 'multipart/mixed';
 const document = { openapi: '3.2.0', paths: { '/body': { post: { requestBody: { content: { 'multipart/mixed': {}, 'application/json': {} } } } } } };
 const source = { toJS: () => document };
 const system = { specSelectors: { specJson: () => source }, oas3Selectors: { requestContentType: () => selected, activeExamplesMember: () => null } };
 const execute = feature.statePlugins.spec.wrapActions.executeRequest(value => { calls++; return value; }, system);
 const request = { pathName: '/body', method: 'post' };
 assert.equal(execute(request).payload.code, 'openapi.ui.multipart');
 assert.equal(calls, 0);
 selected = 'application/json';
 assert.equal(execute(request), request);
 assert.equal(calls, 1);
 assert.equal(execute({ ...request, requestContentType: 'multipart/mixed' }).payload.code, 'openapi.ui.multipart');
 assert.equal(calls, 1);
});
