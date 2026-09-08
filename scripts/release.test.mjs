import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, readFileSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, delimiter } from 'node:path';
import { fileURLToPath } from 'node:url';
import { version, compare, branchAllowed, validate } from './release.mjs';

const script = fileURLToPath(new URL('./release.mjs', import.meta.url));
const gates = ['test (ubuntu-24.04)', 'test (macos-15)', 'remote (ubuntu-24.04)', 'remote (macos-15)', 'fuzz', 'security', 'browser', 'Release policy', 'Required checks'];

// 每个用例使用真实的隔离 Git 仓库；只有 GitHub API 被替身隔离。
// Exercise real Git history while substituting only the external GitHub API.
function fixture(body) {
  const root = mkdtempSync(join(tmpdir(), 'release-policy-'));
  const cwd = process.cwd();
  const git = (...args) => execFileSync('git', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  try {
    mkdirSync(join(root, 'repo')); mkdirSync(join(root, 'bin'));
    process.chdir(root); git('init', '--bare', 'remote.git'); process.chdir(join(root, 'repo'));
    git('init', '-b', 'main'); git('config', 'user.name', 'Release test'); git('config', 'user.email', 'release@example.test');
    writeFileSync('go.mod', 'module github.com/go-devtools/openapi\n\ngo 1.27.1\n');
    writeFileSync('VERSION', 'v0.0.1\n');
    for (const name of ['README.md', 'README.zh-cn.md']) writeFileSync(name, 'go get github.com/go-devtools/openapi@v0.0.1\ngo install github.com/go-devtools/openapi/cmd/openapi@v0.0.1\n');
    git('add', '.'); git('commit', '-m', 'feat: initialize release fixture');
    git('remote', 'add', 'origin', join(root, 'remote.git')); git('push', 'origin', 'main:main', 'main:release/0.0');
    const sha = git('rev-parse', 'HEAD');
    const state = join(root, 'github.json'), calls = join(root, 'calls.jsonl');
    const mock = { runs: [{ id: 10, head_sha: sha, status: 'completed', conclusion: 'success', event: 'push' }], jobs: gates.map(name => ({ name, conclusion: 'success' })), release: { draft: false, tag_name: 'v0.0.1' }, releases: [] };
    writeFileSync(state, JSON.stringify(mock));
    writeFileSync(join(root, 'bin', 'gh'), `#!/usr/bin/env node
import fs from 'node:fs';
const args = process.argv.slice(2), data = JSON.parse(fs.readFileSync(process.env.MOCK_STATE));
fs.appendFileSync(process.env.MOCK_CALLS, JSON.stringify(args) + '\\n');
let result = {};
if (args[0] === 'api') {
  const path = args[1];
  if (path.includes('/workflows/ci.yml/runs?')) result = { workflow_runs: data.runs };
  else if (path.includes('/jobs?')) result = { jobs: data.jobs };
  else if (path.includes('/pulls?')) result = [];
  else if (path.endsWith('/pulls')) result = { html_url: 'https://github.com/go-devtools/openapi/pull/1' };
  else if (path.includes('/releases/tags/')) result = data.release;
  else if (path.includes('/releases?')) result = data.releases;
  else throw new Error('Unexpected API: ' + path);
}
console.log(JSON.stringify(result));
`, { mode: 0o755 });
    // 扩展名缺省的模拟工具由本地包声明为 ESM。
    // Treat the extensionless mock command as an ES module.
    writeFileSync(join(root, 'bin', 'package.json'), '{"type":"module"}');
    const env = { ...process.env, PATH: join(root, 'bin') + delimiter + process.env.PATH, GITHUB_REPOSITORY: 'go-devtools/openapi', MOCK_STATE: state, MOCK_CALLS: calls, GITHUB_EVENT_PATH: join(root, 'event.json'), GITHUB_OUTPUT: join(root, 'outputs') };
    const command = (...args) => spawnSync(process.execPath, [script, ...args], { env, encoding: 'utf8' });
    const save = () => writeFileSync(state, JSON.stringify(mock));
    const event = branch => writeFileSync(env.GITHUB_EVENT_PATH, JSON.stringify({ workflow_run: { head_repository: { full_name: env.GITHUB_REPOSITORY }, head_sha: sha, head_branch: branch, event: 'push', conclusion: 'success', html_url: 'https://github.com/go-devtools/openapi/actions/runs/10' } }));
    body({ root, git, sha, mock, save, command, event, calls });
  } finally { process.chdir(cwd); rmSync(root, { recursive: true, force: true }); }
}

test('strict versions reject ambiguous tags, build metadata, unsafe numbers and shell payloads', () => {
  assert.equal(version('v0.0.1').line, '0.0');
  assert.equal(version('v0.1.0-rc.1').prerelease, 'rc');
  assert.equal(version('v0.1.0-dev', true).development, true);
  for (const bad of ['0.0.1', 'v00.0.1', 'v0.0.01', 'v0.0.1-dev', 'v0.0.1-rc.0', 'v0.0.1+build', 'v9007199254740992.0.0', 'v0.0.1; touch secret']) assert.throws(() => version(bad));
  assert(compare('v0.0.10', 'v0.0.9') > 0);
  assert(compare('v0.1.0', 'v0.1.0-rc.10') > 0);
  assert(compare('v0.1.0-rc.10', 'v0.1.0-rc.2') > 0);
  assert(compare('v0.1.0-beta.1', 'v0.1.0-alpha.2') > 0);
});

test('branch policy rejects direct feature promotion, unrelated maintenance and forked release heads', () => {
  for (const [base, head, own] of [['main', 'release/0.0', true], ['develop', 'feature/schema', false], ['release/0.0', 'hotfix/0.0.2', true], ['release/0.1', 'prepare/v0.1.0-rc.1', true], ['hotfix/0.0.2', 'fix/parser', false]]) assert(branchAllowed(base, head, own));
  for (const [base, head, own] of [['main', 'feature/schema', true], ['main', 'release/0.0', false], ['release/0.0', 'hotfix/0.1.2', true], ['release/0.0', 'prepare/v0.0.2', false], ['unknown', 'feature/test', true]]) assert(!branchAllowed(base, head, own));
});

test('release validation requires an annotated matching tag, canonical module and no replace', () => fixture(({ git }) => {
  git('tag', 'v0.0.1'); assert.throws(() => validate('v0.0.1'), /annotated/);
  git('tag', '-d', 'v0.0.1'); git('tag', '-a', 'v0.0.1', '-m', 'Release v0.0.1');
  assert.equal(validate('v0.0.1').product, 'openapi');
  writeFileSync('VERSION', 'v0.0.2\n'); assert.throws(() => validate('v0.0.1'), /differ/);
  writeFileSync('VERSION', 'v0.0.1\n');
  writeFileSync('go.mod', 'module github.com/go-devtools/openapi\ngo 1.27.1\nreplace example.test/unsafe => ../unsafe\n');
  assert.throws(() => validate('v0.0.1'), /replace/);
  writeFileSync('go.mod', 'module github.com/another-owner/openapi\ngo 1.27.1\n');
  assert.throws(() => validate('v0.0.1'), /module path/);
}));

test('stable adapters reject pseudo-versions and prerelease core dependencies', () => fixture(() => {
  for (const dependency of ['v0.0.0-20260908050439-fb93d0a624f7', 'v0.0.1-rc.1']) {
    writeFileSync('go.mod', `module github.com/go-devtools/gin-swagger\ngo 1.27.1\nrequire github.com/go-devtools/openapi ${dependency}\n`);
    assert.throws(() => validate('v0.0.1', false));
  }
  writeFileSync('go.mod', 'module github.com/go-devtools/gin-swagger\ngo 1.27.1\nrequire github.com/go-devtools/openapi v0.0.1\n');
  assert.equal(validate('v0.0.1', false).product, 'gin-swagger');
}));

test('failed or missing exact-commit gates never create release tags', () => fixture(({ command, mock, save, event, git }) => {
  event('main'); mock.runs.unshift({ ...mock.runs[0], id: 11, conclusion: 'failure' }); save();
  assert.notEqual(command('promote').status, 0); assert.equal(git('tag'), '');
  mock.runs.shift(); mock.jobs.pop(); save();
  assert.notEqual(command('promote').status, 0); assert.equal(git('tag'), '');
}));

test('successful main promotion creates one annotated tag and explicitly dispatches publication', () => fixture(({ command, event, git, sha, calls }) => {
  event('main'); const first = command('promote'); assert.equal(first.status, 0, first.stderr);
  assert.equal(git('cat-file', '-t', 'v0.0.1'), 'tag'); assert.equal(git('rev-parse', 'v0.0.1^{commit}'), sha);
  assert(readFileSync(calls, 'utf8').includes('["workflow","run","release.yml","--repo","go-devtools/openapi","--ref","v0.0.1"]'));
  const object = git('rev-parse', 'v0.0.1'); assert.equal(command('promote').status, 0); assert.equal(git('rev-parse', 'v0.0.1'), object);
}));

test('release preparation opens the correct maintenance PR and updates both install commands', () => fixture(({ command, git, calls }) => {
  git('switch', '-c', 'hotfix/0.0.2'); writeFileSync('VERSION', 'v0.0.2-dev\n'); git('commit', '-am', 'chore: prepare next patch'); git('push', 'origin', 'hotfix/0.0.2');
  const prepared = command('prepare', 'v0.0.2'); assert.equal(prepared.status, 0, prepared.stderr);
  assert.equal(git('branch', '--show-current'), 'prepare/v0.0.2');
  assert.equal(readFileSync('VERSION', 'utf8'), 'v0.0.2\n');
  assert(readFileSync('README.md', 'utf8').includes('/cmd/openapi@v0.0.2'));
  assert(readFileSync(calls, 'utf8').includes('prepare/v0.0.2'));
  assert.notEqual(command('prepare', 'v0.0.2').status, 0);
}));

test('published versions seed distinct next-minor and patch branches without moving the tag', () => fixture(({ command, git, sha }) => {
  git('tag', '-a', 'v0.0.1', '-m', 'Release v0.0.1'); git('push', 'origin', 'v0.0.1');
  const result = command('sync', 'v0.0.1'); assert.equal(result.status, 0, result.stderr);
  assert.equal(git('show', 'origin/develop:VERSION'), 'v0.1.0-dev');
  assert.equal(git('show', 'origin/hotfix/0.0.2:VERSION'), 'v0.0.2-dev');
  assert.equal(git('rev-parse', 'v0.0.1^{commit}'), sha);
}));

test('published releases cannot be edited and older maintenance patches cannot become latest', () => fixture(({ command, mock, save, calls }) => {
  assert.notEqual(command('publish', 'v0.0.1').status, 0);
  mock.release.draft = true; mock.releases = [{ tag_name: 'v0.1.0', draft: false, prerelease: false }]; save();
  const result = command('publish', 'v0.0.1'); assert.equal(result.status, 0, result.stderr);
  assert(readFileSync(calls, 'utf8').includes('"--latest=false"'));
}));
