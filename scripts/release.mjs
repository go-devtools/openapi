import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync, appendFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

// 严格解析发布版本，开发占位版本只能用于开发分支。
// Parse immutable SemVer tags; development markers are branch-only.
export function version(value, development = false) {
  const match = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(alpha|beta|rc)\.([1-9]\d*)|-(dev))?$/.exec(value);
  assert(match && (development || !match[6]), `Invalid release version: ${value}`);
  const [major, minor, patch] = match.slice(1, 4).map(Number);
  assert([major, minor, patch].every(Number.isSafeInteger), 'Version numbers exceed safe bounds');
  return { value, major, minor, patch, line: `${major}.${minor}`, prerelease: match[4] || '', sequence: Number(match[5] || 0), development: !!match[6] };
}

// 按语义版本顺序比较正式版本与预发布版本。
// Compare stable and prerelease versions without lexical-number mistakes.
export function compare(left, right) {
  const a = version(left), b = version(right);
  for (const key of ['major', 'minor', 'patch']) if (a[key] !== b[key]) return Math.sign(a[key] - b[key]);
  const rank = { alpha: 0, beta: 1, rc: 2, '': 3 };
  return Math.sign(rank[a.prerelease] - rank[b.prerelease]) || Math.sign(a.sequence - b.sequence);
}

// 限制合并方向，防止未稳定功能直接进入正式主分支。
// Restrict integration directions while retaining topic branches for contributors.
export function branchAllowed(base, head, sameRepository = true) {
  if (base === 'main') return sameRepository && /^release\/\d+\.\d+$/.test(head);
  if (base === 'develop') return /^(feature|fix|docs|chore|refactor|test|ci|build|dependabot|sync)\//.test(head);
  if (/^release\/\d+\.\d+$/.test(base)) {
    const match = /^(?:prepare\/v|hotfix\/)(\d+\.\d+)\.\d+(?:-(?:alpha|beta|rc)\.\d+)?$/.exec(head);
    return sameRepository && !!match && base === `release/${match[1]}`;
  }
  if (/^hotfix\/\d+\.\d+\.\d+$/.test(base)) return /^(fix|docs|test|chore|dependabot)\//.test(head);
  return false;
}

// 使用参数数组执行命令，避免版本与分支名称进入 shell。
// Execute arguments directly instead of interpolating user input into a shell.
export function run(command, args, options = {}) { return execFileSync(command, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], ...options }).trim(); }
const git = (...args) => run('git', args);
const present = ref => spawnSync('git', ['rev-parse', '--verify', '--quiet', ref], { stdio: 'ignore' }).status === 0;
const ancestor = (a, b) => spawnSync('git', ['merge-base', '--is-ancestor', a, b], { stdio: 'ignore' }).status === 0;
const api = (path, body) => JSON.parse(run('gh', ['api', path, ...(body ? ['--method', 'POST', '--input', '-'] : [])], body ? { input: JSON.stringify(body), stdio: ['pipe', 'pipe', 'pipe'] } : {}));
const repository = () => { const name = process.env.GITHUB_REPOSITORY; assert(/^go-devtools\/(openapi|gin-swagger)$/.test(name), 'Unexpected release repository'); return name; };
const output = (key, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${key}=${value}\n`); };
const push = ref => run('git', ['-c', 'credential.helper=!gh auth git-credential', 'push', 'origin', ref]);
// 显式读取全部工作线，避免单分支 checkout 隐藏发布祖先。
// Fetch all branch ancestry even after a single-branch checkout.
const fetch = () => git('fetch', 'origin', '+refs/heads/*:refs/remotes/origin/*', '--tags');

// 发布线内版本严格递增，旧维护线仍可独立发布补丁。
// Require increasing versions within each independently maintained release line.
function nextVersion(tag) {
  const parsed = version(tag);
  for (const existing of git('tag', '--list', 'v*').split('\n').filter(Boolean)) {
    if (!/^v\d+\.\d+\.\d+(?:-(?:alpha|beta|rc)\.\d+)?$/.test(existing)) continue;
    if (version(existing).line === parsed.line) assert(compare(tag, existing) > 0, `Version must follow ${existing}`);
  }
}

// 检查版本、模块路径、发布线与不可移动的附注标签。
// Validate module identity, release lineage and an immutable annotated tag.
export function validate(tag, requireTag = true) {
  const parsed = version(tag);
  assert.equal(readFileSync('VERSION', 'utf8').trim(), tag, 'VERSION and release tag differ');
  const manifest = JSON.parse(run('go', ['mod', 'edit', '-json']));
  const product = manifest.Module.Path.split('/').filter(part => !/^v\d+$/.test(part)).at(-1);
  assert(['openapi', 'gin-swagger'].includes(product));
  assert.equal(manifest.Module.Path, `github.com/go-devtools/${product}${parsed.major >= 2 ? `/v${parsed.major}` : ''}`, 'Major version requires the matching Go module path');
  assert.equal((manifest.Replace || []).length, 0, 'Release modules cannot use replace');
  if (product === 'gin-swagger') {
    const core = manifest.Require.find(item => item.Path === 'github.com/go-devtools/openapi');
    assert(core, 'The adapter must pin the public core');
    const coreVersion = version(core.Version);
    assert(parsed.prerelease || !coreVersion.prerelease, 'Stable adapters require a stable core tag');
  }
  if (requireTag) {
    assert.equal(git('cat-file', '-t', `refs/tags/${tag}`), 'tag', 'Release tags must be annotated');
    assert.equal(git('rev-parse', `${tag}^{commit}`), git('rev-parse', 'HEAD'), 'Checkout and tag differ');
    assert(present(`refs/remotes/origin/release/${parsed.line}`), 'Missing release maintenance branch');
    assert(ancestor('HEAD', 'origin/main') || ancestor('HEAD', `origin/release/${parsed.line}`), 'Tag is not on a reviewed release lineage');
  }
  return { ...parsed, product, module: manifest.Module.Path, commit: git('rev-parse', 'HEAD') };
}

// 只接受同一提交最近一次真实 CI 的成功结果。
// Require the latest published-source CI run for this exact commit to pass.
export function requireCI(repo, commit) {
  const runs = api(`repos/${repo}/actions/workflows/ci.yml/runs?head_sha=${commit}&per_page=100`).workflow_runs
    .filter(item => ['push', 'workflow_dispatch'].includes(item.event) && item.head_sha === commit)
    .sort((a, b) => b.id - a.id);
  assert(runs.length && runs[0].status === 'completed' && runs[0].conclusion === 'success', 'The latest source CI has not passed for this commit');
  const jobs = api(`repos/${repo}/actions/runs/${runs[0].id}/jobs?per_page=100`).jobs;
  const required = ['test (ubuntu-24.04)', 'test (macos-15)', 'remote (ubuntu-24.04)', 'remote (macos-15)', 'fuzz', 'security', 'browser', 'Release policy', 'Required checks'];
  for (const name of required) assert(jobs.some(job => job.name === name && job.conclusion === 'success'), `Missing successful CI gate: ${name}`);
  return runs[0].html_url;
}

// 创建可审查的版本与回流 PR，并显式启动令牌事件不会自动触发的 CI。
// Open reviewable automation PRs and explicitly dispatch their CI.
function openPR(repo, head, base, title, body, dispatch = true) {
  const existing = api(`repos/${repo}/pulls?state=open&head=go-devtools:${encodeURIComponent(head)}&base=${encodeURIComponent(base)}`);
  if (existing.length) return existing[0].html_url;
  const pull = api(`repos/${repo}/pulls`, { head, base, title, body, maintainer_can_modify: false });
  if (dispatch) run('gh', ['workflow', 'run', 'ci.yml', '--repo', repo, '--ref', head]);
  console.log(pull.html_url);
  return pull.html_url;
}

// 从下一版本或修复分支准备版本号，发布前始终经过 PR。
// Prepare a version bump from development or a hotfix through a PR.
function prepare(tag) {
  const parsed = version(tag), repo = repository(), line = `release/${parsed.line}`;
  fetch();
  assert(!present(`refs/tags/${tag}`), 'This version already exists; publish a new version instead');
  nextVersion(tag);
  const hotfix = `hotfix/${parsed.major}.${parsed.minor}.${parsed.patch}`;
  const source = present(`refs/remotes/origin/${hotfix}`) ? hotfix : present(`refs/remotes/origin/${line}`) ? line : 'develop';
  const head = `prepare/${tag}`;
  assert(!present(`refs/remotes/origin/${head}`), 'Preparation branch exists; continue its existing PR');
  if (!present(`refs/remotes/origin/${line}`)) push(`refs/remotes/origin/${source}:refs/heads/${line}`);
  git('switch', '--create', head, `origin/${source}`);
  writeFileSync('VERSION', tag + '\n');
  const module = JSON.parse(run('go', ['mod', 'edit', '-json'])).Module.Path;
  for (const name of ['README.md', 'README.zh-cn.md']) {
    const previous = readFileSync(name, 'utf8');
    writeFileSync(name, previous.replaceAll(new RegExp(`(${module.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?:/cmd/[^@\\s]+)?@)v\\d+\\.\\d+\\.\\d+(?:-(?:alpha|beta|rc)\\.\\d+)?`, 'g'), `$1${tag}`));
  }
  validate(tag, false);
  git('add', 'VERSION', 'README.md', 'README.zh-cn.md');
  git('commit', '-m', `chore: prepare ${tag}`);
  push(`HEAD:refs/heads/${head}`);
  openPR(repo, head, line, `chore: prepare ${tag}`, `Prepare **${tag}** from \`${source}\`.\n\nReview the changes and merge after Required checks passes. Successful release-line CI opens the stable promotion PR, or publishes a prerelease / older maintenance patch. Tags and GitHub release files are created only after the exact source commit passes CI.\n\nGo module: \`${module}@${tag}\`.`);
}

// 成功 CI 后推动稳定分支，或创建附注标签并显式调用发版工作流。
// Promote reviewed releases and dispatch publication after successful source CI.
function promote() {
  const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8'));
  const source = event.workflow_run, repo = repository();
  assert(source && source.head_repository.full_name === repo && source.conclusion === 'success');
  if (!['push', 'workflow_dispatch'].includes(source.event) || !/^(main|release\/\d+\.\d+)$/.test(source.head_branch)) return;
  fetch();
  assert.equal(git('rev-parse', `origin/${source.head_branch}`), source.head_sha, 'A newer commit superseded this CI result');
  git('switch', '--detach', source.head_sha);
  const tag = readFileSync('VERSION', 'utf8').trim();
  if (version(tag, true).development || present(`refs/tags/${tag}`)) return;
  nextVersion(tag);
  const parsed = validate(tag, false);
  requireCI(repo, source.head_sha);
  const main = version(git('show', 'origin/main:VERSION'), true);
  if (source.head_branch !== 'main' && !ancestor(source.head_sha, 'origin/main') && !parsed.prerelease && (parsed.major > main.major || (parsed.major === main.major && parsed.minor >= main.minor))) {
    openPR(repo, source.head_branch, 'main', `chore: release ${tag}`, `Promote the reviewed **${tag}** release line to main.\n\nSource CI: ${source.html_url}\n\nAfter merge, main CI must pass before automation creates the immutable tag and release assets.`, false);
    return;
  }
  assert(source.head_branch === 'main' || source.head_branch === `release/${parsed.line}`);
  assert(present(`refs/remotes/origin/release/${parsed.line}`), 'Missing release line');
  git('tag', '-a', tag, '-m', `Release ${tag}`, source.head_sha);
  push(`refs/tags/${tag}`);
  run('gh', ['workflow', 'run', 'release.yml', '--repo', repo, '--ref', 'main', '-f', `tag=${tag}`]);
  console.log(`Created ${tag} at ${source.head_sha}; publication dispatched`);
}

// 发布后为当前维护版本预建修复分支，并将修复通过 PR 回流开发线。
// Seed the next hotfix and back-merge published changes through a reviewed PR.
function sync(tag) {
  const parsed = version(tag), repo = repository();
  if (parsed.prerelease) return;
  fetch();
  const release = api(`repos/${repo}/releases/tags/${tag}`);
  assert(!release.draft && release.tag_name === tag, 'Only published releases may seed workstreams');
  const fix = `hotfix/${parsed.major}.${parsed.minor}.${parsed.patch + 1}`;
  if (!present(`refs/remotes/origin/${fix}`)) {
    git('switch', '--create', fix, tag);
    writeFileSync('VERSION', `v${parsed.major}.${parsed.minor}.${parsed.patch + 1}-dev\n`);
    git('add', 'VERSION'); git('commit', '-m', `chore: prepare ${fix}`); push(`HEAD:refs/heads/${fix}`);
    run('gh', ['workflow', 'run', 'ci.yml', '--repo', repo, '--ref', fix]);
  }
  if (!present('refs/remotes/origin/develop')) {
    git('switch', '--create', 'develop', tag);
    writeFileSync('VERSION', `v${parsed.major}.${parsed.minor + 1}.0-dev\n`);
    git('add', 'VERSION'); git('commit', '-m', 'chore: prepare the next development version'); push('HEAD:refs/heads/develop');
    run('gh', ['workflow', 'run', 'ci.yml', '--repo', repo, '--ref', 'develop']);
    return;
  }
  if (ancestor(tag, 'origin/develop')) return;
  const head = `sync/${tag}-to-develop`;
  if (present(`refs/remotes/origin/${head}`)) return;
  git('switch', '--create', head, 'origin/develop');
  const development = git('show', 'origin/develop:VERSION');
  const merge = spawnSync('git', ['merge', '--no-commit', '--no-ff', tag], { encoding: 'utf8' });
  const conflicts = git('diff', '--name-only', '--diff-filter=U').split('\n').filter(Boolean);
  if (merge.status !== 0 && conflicts.some(path => path !== 'VERSION')) {
    git('merge', '--abort'); git('reset', '--hard', tag);
    push(`HEAD:refs/heads/${head}`);
    openPR(repo, head, 'develop', `chore: merge ${tag} into develop`, `Backport **${tag}**. Merge conflicts require maintainer resolution. Preserve develop's next-version marker and resolve code conflicts before merging.`, false);
    return;
  }
  assert(merge.status === 0 || conflicts.length === 1 && conflicts[0] === 'VERSION', merge.stderr);
  writeFileSync('VERSION', development + '\n'); git('add', 'VERSION');
  git('commit', '-m', `chore: merge ${tag} into develop`); push(`HEAD:refs/heads/${head}`);
  openPR(repo, head, 'develop', `chore: merge ${tag} into develop`, `Bring the published **${tag}** changes into develop while preserving its next-version marker. Review and merge after Required checks passes.`);
}

// 按入口执行；被测试导入时不产生任何外部写入。
// Dispatch commands only when executed, never when imported by tests.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [command, argument] = process.argv.slice(2);
  if (command === 'validate') {
    const result = validate(argument); requireCI(repository(), result.commit);
    output('tag', result.value); output('product', result.product); output('prerelease', !!result.prerelease); output('commit', result.commit);
    console.log(JSON.stringify(result));
  } else if (command === 'policy') {
    version(readFileSync('VERSION', 'utf8').trim(), true);
    const event = process.env.GITHUB_EVENT_PATH ? JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8')) : {};
    if (event.pull_request) assert(branchAllowed(event.pull_request.base.ref, event.pull_request.head.ref, event.pull_request.base.repo.full_name === event.pull_request.head.repo.full_name), 'Unsupported branch flow; see CONTRIBUTING.md');
    console.log('Release policy passed');
  } else if (command === 'prepare') prepare(argument);
  else if (command === 'promote') promote();
  else if (command === 'sync') sync(argument);
  else if (command === 'publish') {
    const parsed = version(argument), repo = repository();
    const release = JSON.parse(run('gh', ['release', 'view', argument, '--repo', repo, '--json', 'isDraft,tagName']));
    assert(release.isDraft && release.tagName === argument, 'Published releases are immutable; do not rerun publication');
    const latest = api(`repos/${repo}/releases?per_page=100`).filter(item => !item.draft && !item.prerelease).map(item => item.tag_name).filter(item => /^v\d+\.\d+\.\d+$/.test(item)).sort((a, b) => compare(b, a))[0];
    run('gh', ['release', 'edit', argument, '--repo', repo, '--draft=false', `--prerelease=${!!parsed.prerelease}`, `--latest=${!parsed.prerelease && (!latest || compare(argument, latest) > 0)}`]);
  } else throw new Error('Usage: release.mjs {policy|validate TAG|prepare TAG|promote|publish TAG|sync TAG}');
}
