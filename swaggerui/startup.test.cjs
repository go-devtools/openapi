const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

// Capture Swagger UI configuration from the actual startup script without loading a browser or external specifications.
function start(query, definitions = true) {
  let captured;
  const bundle = (options) => { captured = options; return {}; };
  bundle.presets = { apis: {} };
  const window = { location: { href: 'http://127.0.0.1:18080/docs/' + query }, OpenAPIDisplayNames: {}, OpenAPINativeExamples: {}, OpenAPINativeCompatibility: {} };
  vm.runInNewContext(fs.readFileSync(__dirname + '/startup.js', 'utf8'), { window, URL, SwaggerUIBundle: bundle, SwaggerUIStandalonePreset: {} });
  const options = { queryConfigEnabled: false, validatorUrl: null, supportedSubmitMethods: [], url: './openapi.json' };
  if (definitions) {
    delete options.url;
    options.urls = [{ name: 'All endpoints', url: './groups/all.json' }, { name: 'Legacy · Deprecated', url: './groups/legacy.json' }];
    options['urls.primaryName'] = 'All endpoints';
  }
  window.OpenAPIStart(options);
  return captured;
}

// Restore the same document on deep links and refreshes instead of returning to the default overview.
test('Restore registered definitions while preserving fixed specification URLs', () => {
  const options = start('?urls.primaryName=' + encodeURIComponent('Legacy · Deprecated') + '#/Legacy/example');
  assert.equal(options['urls.primaryName'], 'Legacy · Deprecated');
  assert.equal(options.urls[1].url, './groups/legacy.json');
});

// Restore groups without arbitrary query configuration that could replace specifications, validators, or submission methods.
test('Reject unknown definitions and other query overrides', () => {
  const options = start('?urls.primaryName=https://evil.test/spec&url=https://evil.test/spec&configUrl=https://evil.test/config&validatorUrl=https://evil.test&supportedSubmitMethods=get');
  assert.equal(options['urls.primaryName'], 'All endpoints');
  assert.equal(options.url, undefined);
  assert.equal(options.configUrl, undefined);
  assert.equal(options.validatorUrl, null);
  assert.equal(options.queryConfigEnabled, false);
  assert.deepEqual(options.supportedSubmitMethods, []);
});

// Keep a single document's entry point unchanged by group query parameters.
test('Single documents retain their explicit local URL', () => {
  const options = start('?urls.primaryName=Any&url=https://evil.test/spec', false);
  assert.equal(options.url, './openapi.json');
  assert.equal(options['urls.primaryName'], undefined);
});
