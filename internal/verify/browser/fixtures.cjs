const { test: base, expect } = require('@playwright/test');
const { execFile, spawn } = require('node:child_process');
const { promisify } = require('node:util');
const { mkdtemp, rm } = require('node:fs/promises');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');

// Own the compiled service and ephemeral port; shut down only the process created by this fixture.
// 独占编译出的服务与临时端口，只关闭本 fixture 启动的进程。
const test = base.extend({
 server: [async ({}, use) => {
  const root = resolve(__dirname, '../../..');
  const directory = await mkdtemp(join(tmpdir(), 'openapi-browser-server-'));
  let processHandle;
  try {
   const binary = join(directory, 'browser-server');
   await promisify(execFile)('go', ['build', '-o', binary, './internal/verify/testdata/browser'], { cwd: root, env: { ...process.env, GOWORK: 'off', GOTOOLCHAIN: 'local' }, timeout: 120000 });
   processHandle = spawn(binary, [], { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] });
   let errors = '';
   processHandle.stderr.on('data', data => { errors += data; });
   const url = await new Promise((resolveURL, reject) => {
    let output = '';
    const deadline = setTimeout(() => reject(new Error('Browser fixture did not start: ' + errors)), 30000);
    processHandle.once('exit', code => { clearTimeout(deadline); reject(new Error('Browser fixture exited before readiness: ' + code + ' ' + errors)); });
    processHandle.stdout.on('data', data => {
     output += data;
     const line = output.split('\n').find(value => value.startsWith('{"url":'));
     if (line) { clearTimeout(deadline); resolveURL(JSON.parse(line).url); }
    });
   });
   expect(new URL(url).hostname).toBe('127.0.0.1');
   await use({ url });
  } finally {
   if (processHandle && processHandle.exitCode === null) {
    const stopped = new Promise(resolveExit => processHandle.once('exit', resolveExit));
    processHandle.kill('SIGINT');
    const force = setTimeout(() => processHandle.kill('SIGKILL'), 5000);
    await stopped;
    clearTimeout(force);
   }
   await rm(directory, { recursive: true, force: true });
  }
 }, { scope: 'worker', timeout: 150000 }],
 baseURL: async ({ server }, use) => { await use(server.url); },
 networkGuard: [async ({ context, server }, use) => {
  const unexpected = [];
  const errors = [];
  const origin = new URL(server.url).origin;
  await context.route('**/*', async route => {
   if (new URL(route.request().url()).origin === origin) await route.continue();
   else { unexpected.push(route.request().url()); await route.abort('blockedbyclient'); }
  });
  context.on('page', page => {
   page.on('pageerror', error => { errors.push(error.message); });
   page.on('console', message => { if (message.type() === 'error') errors.push(message.text()); });
  });
  await use();
  expect(unexpected, 'No browser request may leave the local fixture').toEqual([]);
  expect(errors, 'The rendered UI must have no console or runtime errors').toEqual([]);
 }, { auto: true }]
});
module.exports = { test, expect };
