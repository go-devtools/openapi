const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

// 用真实启动脚本捕获传给 Swagger UI 的配置，不加载浏览器或外部规范。
function start(query, definitions = true) {
  let captured;
  const bundle = (options) => { captured = options; return {}; };
  bundle.presets = { apis: {} };
  const window = { location: { href: 'http://127.0.0.1:18080/docs/' + query }, OpenAPIDisplayNames: {} };
  vm.runInNewContext(fs.readFileSync(__dirname + '/startup.js', 'utf8'), { window, URL, SwaggerUIBundle: bundle, SwaggerUIStandalonePreset: {} });
  const options = { queryConfigEnabled: false, validatorUrl: null, supportedSubmitMethods: [], url: './openapi.json' };
  if (definitions) {
    delete options.url;
    options.urls = [{ name: '全部接口', url: './groups/all.json' }, { name: '兼容接口 · Deprecated', url: './groups/legacy.json' }];
    options['urls.primaryName'] = '全部接口';
  }
  window.OpenAPIStart(options);
  return captured;
}

// 分类深链接和刷新恢复同一文档，不会回到默认总览。
test('恢复已注册分类，并保留固定规范地址', () => {
  const options = start('?urls.primaryName=' + encodeURIComponent('兼容接口 · Deprecated') + '#/兼容接口/example');
  assert.equal(options['urls.primaryName'], '兼容接口 · Deprecated');
  assert.equal(options.urls[1].url, './groups/legacy.json');
});

// 恢复分类不启用任意查询配置，攻击者不能替换规范、验证器或提交方法。
test('拒绝未知分类和其他查询串覆盖项', () => {
  const options = start('?urls.primaryName=https://evil.test/spec&url=https://evil.test/spec&configUrl=https://evil.test/config&validatorUrl=https://evil.test&supportedSubmitMethods=get');
  assert.equal(options['urls.primaryName'], '全部接口');
  assert.equal(options.url, undefined);
  assert.equal(options.configUrl, undefined);
  assert.equal(options.validatorUrl, null);
  assert.equal(options.queryConfigEnabled, false);
  assert.deepEqual(options.supportedSubmitMethods, []);
});

// 单份文档不会因为分类查询参数改变加载入口。
test('单文档继续使用显式本地地址', () => {
  const options = start('?urls.primaryName=任意&url=https://evil.test/spec', false);
  assert.equal(options.url, './openapi.json');
  assert.equal(options['urls.primaryName'], undefined);
});
