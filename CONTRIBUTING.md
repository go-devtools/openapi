# Contributing

[简体中文](CONTRIBUTING.zh-cn.md)

Both `go-devtools/openapi` and `go-devtools/gin-swagger` begin at **v0.0.1** and publish independently. Go module versions come from immutable Git tags; `VERSION` records a branch's intended release. The adapter pins a real released core version. Never use a local `replace` in a submitted module.

## Branches and pull requests

| Branch | Purpose | Changes arrive from |
| --- | --- | --- |
| `main` | Stable integration and default branch | `release/M.m` PRs |
| `develop` | Next minor/major development, initially `v0.1.0-dev` | `feature/*`, `fix/*`, `docs/*`, `chore/*`, `refactor/*`, `test/*`, `ci/*`, `build/*`, `dependabot/*`, `sync/*` PRs |
| `release/M.m` | Stabilization and maintenance for one version line, initially `release/0.0` | `prepare/vM.m.p` and matching `hotfix/M.m.p` PRs |
| `hotfix/M.m.p` | Isolated preparation of the next patch, initially `hotfix/0.0.2` | `fix/*`, `docs/*`, `test/*`, `chore/*`, `dependabot/*` PRs |

Create topic branches from their intended target. A next-version feature starts from `develop`; a current-version correction starts from the current hotfix branch. Contributors can use forks for topic PRs. Release and promotion branches must belong to this repository. Main, develop and release lines require PRs and **Required checks**; force pushes and deletion are disabled. Version tags must never be moved or deleted.

Use English conventional commit subjects: `feat: add ...`, `fix: handle ...`, `docs: explain ...`, `ci: verify ...`. Keep changes reviewable. Add concise Chinese and English comments when introducing logic that needs explanation. Public documentation and OpenAPI example text remain English, with corresponding Chinese README documentation.

## Validate locally

Use Go 1.27.1 and Node 24.20.0:

```sh
GOWORK=off go mod download
GOWORK=off make dev
node --test scripts/release.test.mjs
node scripts/release.mjs policy
```

The full independent CI additionally runs race, vet, bounded fuzz, vulnerability and offline browser checks. Push and manual runs verify a separate consumer at the exact published commit. Fork PRs need no private-module credentials; no privileged `pull_request_target` workflow is used. See [CI details](docs/ci.md).

## Prepare and publish a version

1. Finish changes on `develop` for a new version line, or the next `hotfix/M.m.p` for a patch. Publish a needed core version before updating the adapter dependency.
2. Run **Prepare release** from `main`, supplying a new version such as `v0.0.2` or `v0.1.0-rc.1`. It chooses the matching hotfix, existing release line, or develop in that order; opens a version-bump PR into `release/M.m`; and explicitly starts CI. Review the diff and merge after **Required checks** passes. Creating a new line snapshots develop; later development continues separately.
3. Successful release-line CI opens a PR into `main` for the current/new stable line. Review and merge it. Main CI must pass at the exact merged commit before automation creates an annotated tag. Prereleases and older maintenance lines publish from their release branch after the same source gates. The workflow never retags an existing version.
4. **Release** verifies the tag and CI, installs the public module without credentials or workspace on Linux and macOS, builds six CLI archives, and uploads a draft with source and SHA-256 checksums. The existing publishing job downloads the actual draft assets and passes them to read-only verification jobs. Those jobs execute the binaries on Linux/amd64, macOS/arm64 and Windows/amd64 before publication. This does not claim full Windows library-test coverage. The largest stable version becomes Latest; prereleases and older maintenance patches do not replace it.
5. Publication seeds the next hotfix branch. If develop does not exist it seeds the next minor; otherwise it opens a back-merge PR when needed. Real code conflicts require maintainer resolution. Merge fixes back into develop before continuing feature work. A failed sync can be retried independently without changing the published tag or files.

All publication steps use the repository's scoped `GITHUB_TOKEN`; no personal token is required. In repository **Settings → Actions → General**, allow GitHub Actions to create pull requests. Automated Git writes do not normally trigger another workflow, so the scripts explicitly dispatch CI and Release. Branch protection still requires reviewable PRs and checks; no automatic PR approval or merge is enabled.

## Recovery and compatibility

- A failed build or smoke check leaves a draft. Rerun the failed jobs after fixing an infrastructure issue. Source changes require a new version; never replace the commit under a published tag.
- If automatic tagging is interrupted, rerun the source CI or **Promote release** job. If the tag already exists but publication never started, dispatch **Release** from `main` and supply that existing tag. Controls are loaded from the workflow commit; every source checkout remains pinned to the validated tag. This repairs publication infrastructure without rewriting module versions.
- Retry a failed sync job from its run after resolving the cause. Existing workstream branches are preserved.
- `v0` is an evolving API: pin versions and review release notes. Patch releases should contain compatible fixes; schedule intentional API changes on develop for a new minor. A future `v2` or later must use the matching `/vN` Go module path.
- Release archives include the project MIT license and third-party notices. They complement normal `go get`; application code does not depend on downloading a binary archive.
