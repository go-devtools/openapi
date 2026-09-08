import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync, mkdirSync, mkdtempSync, readdirSync } from 'node:fs';
import { join, basename } from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';

// 校验所有发布文件，防止缺少未在本机执行的平台附件。
// Verify every archive, including platforms outside the native smoke matrix.
export function verifyAssets(tag, dir, product) {
  assert(/^v\d+\.\d+\.\d+(?:-(?:alpha|beta|rc)\.\d+)?$/.test(tag));
  assert(['openapi', 'gin-swagger'].includes(product));
  const names = ['linux', 'darwin', 'windows'].flatMap(os => ['amd64', 'arm64'].map(arch => `${product}_${tag.slice(1)}_${os}_${arch}.${os === 'windows' ? 'zip' : 'tar.gz'}`));
  names.push(`${product}_${tag.slice(1)}_source.tar.gz`);
  const entries = readFileSync(join(dir, 'checksums.txt'), 'utf8').trim().split('\n');
  assert.equal(entries.length, names.length, 'Incomplete or unexpected checksum manifest');
  for (const name of names) {
    const rows = entries.filter(line => line.endsWith(`  ${name}`));
    assert.equal(rows.length, 1, `Missing or repeated checksum for ${name}`);
    assert.equal(createHash('sha256').update(readFileSync(join(dir, name))).digest('hex'), rows[0].split(' ')[0], `Checksum mismatch: ${name}`);
  }
  return names;
}

// 用参数数组调用工具，保留失败的原始退出状态。
// Invoke tools without shell interpolation and preserve failures.
const run = (cmd, args, options = {}) => execFileSync(cmd, args, { encoding: 'utf8', ...options }).trim();

// 随可执行文件分发实际编译依赖与嵌入资源的许可证。
// Package licenses for compiled dependencies and embedded upstream assets.
function notices() {
  const product = basename(run('go', ['list', '-m']));
  const dirs = new Set(run('go', ['list', '-deps', '-f', '{{with .Module}}{{.Dir}}{{end}}', `./cmd/${product}`]).split('\n').filter(Boolean));
  dirs.add(run('go', ['env', 'GOROOT']));
  const sections = [];
  for (const dir of dirs) {
    const files = readdirSync(dir).filter(name => /^(LICENSE|COPYING|NOTICE)(\.|$)/i.test(name));
    assert(files.length, `Missing license for ${dir}`);
    for (const name of files) sections.push(`=== ${basename(dir)} / ${name} ===\n${readFileSync(join(dir, name), 'utf8')}`);
    // 核心内嵌 Swagger UI；适配器的依赖副本也包含这些资源。
    // The core embeds Swagger UI, including when consumed by the adapter.
    if (basename(dir).startsWith('openapi')) {
      for (const name of ['swaggerui/NOTICE', 'swaggerui/assets/LICENSE', 'swaggerui/assets/NOTICE', 'swaggerui/assets/swagger-ui-bundle.js.LICENSE.txt', 'swaggerui/assets/swagger-ui-standalone-preset.js.LICENSE.txt']) {
        sections.push(`=== openapi / ${name} ===\n${readFileSync(join(dir, name), 'utf8')}`);
      }
    }
  }
  mkdirSync('release-notices', { recursive: true });
  writeFileSync('release-notices/THIRD_PARTY_NOTICES.txt', sections.join('\n\n'));
}

// 下载真实草稿附件并验证哈希、版本与本机校验命令。
// Smoke-test actual draft assets before making the release public.
function smoke(tag) {
  assert(/^v\d+\.\d+\.\d+(?:-(?:alpha|beta|rc)\.\d+)?$/.test(tag));
  const repo = process.env.GITHUB_REPOSITORY;
  assert(/^go-devtools\/(openapi|gin-swagger)$/.test(repo));
  const product = repo.split('/')[1];
  const os = { linux: 'linux', darwin: 'darwin', win32: 'windows' }[process.platform];
  const arch = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  assert(os && arch, 'Unsupported native smoke runner');
  const archive = `${product}_${tag.slice(1)}_${os}_${arch}.${os === 'windows' ? 'zip' : 'tar.gz'}`;
  const dir = process.env.RELEASE_ASSET_DIR || mkdtempSync(join(tmpdir(), 'release-smoke-'));
  if (!process.env.RELEASE_ASSET_DIR) run('gh', ['release', 'download', tag, '--repo', repo, '--dir', dir, '--pattern', archive, '--pattern', 'checksums.txt']);
  if (process.env.RELEASE_ASSET_DIR) verifyAssets(tag, dir, product);
  const expected = readFileSync(join(dir, 'checksums.txt'), 'utf8').split('\n').find(line => line.endsWith(`  ${archive}`))?.split(' ')[0];
  assert(expected, 'Archive is missing from checksums');
  assert.equal(createHash('sha256').update(readFileSync(join(dir, archive))).digest('hex'), expected);
  run('tar', ['-xf', join(dir, archive), '-C', dir]);
  const binary = join(dir, product + (os === 'windows' ? '.exe' : ''));
  const actual = JSON.parse(run(binary, ['version']));
  assert.equal(actual.version, tag);
  assert.equal(actual.module, `github.com/${repo}`);
  assert.equal(actual.revision, process.env.RELEASE_COMMIT);
  if (product === 'gin-swagger') assert(/^v\d+\.\d+\.\d+/.test(actual.core));
  assert(readFileSync(join(dir, 'THIRD_PARTY_NOTICES.txt'), 'utf8').includes('Swagger UI'));
  const spec = join(dir, 'valid.json');
  writeFileSync(spec, JSON.stringify({ openapi: '3.2.0', info: { title: 'Release smoke', version: '1' }, paths: {} }));
  run(binary, ['check', '--spec', spec]);
  writeFileSync(spec, '{broken');
  assert.throws(() => run(binary, ['check', '--spec', spec], { stdio: ['ignore', 'pipe', 'pipe'] }));
  console.log(JSON.stringify({ archive, sha256: expected, ...actual, nativeCheck: 'passed' }));
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  if (process.argv[2] === 'notices') notices();
  else if (process.argv[2] === 'smoke') smoke(process.argv[3]);
  else throw new Error('Usage: release-assets.mjs {notices|smoke TAG}');
}
