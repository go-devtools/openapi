# Independent CI

Each repository checks out only its own source. `GOWORK=off` and `GOTOOLCHAIN=local` prevent a local workspace or automatic toolchain download from masking module requirements. No `replace` directive is permitted in the manifest or selected module graph.

## Toolchain

The workflow pins Go **1.27.1**, Gin **v1.12.0** (adapter only), Node **24.20.0**, npm **11.19.0**, Playwright **1.63.0**, and govulncheck **v1.7.0**. Official Actions use full commit SHAs. Node packages use the committed npm lockfile. Browser binaries come from the pinned Playwright distribution.

The runtime matrix executes on `ubuntu-24.04` (amd64) and `macos-15` (arm64). It does not claim Windows runtime verification. Go 1.27.1 and Gin v1.12.0 were also the latest stable versions when this matrix was configured; no newer combination is claimed. A future version must be added explicitly and actually tested.

Every job starts with separate module and build cache paths under runner temporary storage. Workflow jobs do not restore a shared Go cache. Evidence records actual `GOOS`, `GOARCH`, toolchain, CGO and module versions. Security checking fails on reachable vulnerabilities; module-only advisories are still printed and must not be described as absent.

## Local commands

Use the pinned tools; both product modules are public and require no credentials:

```sh
GOWORK=off go mod download
GOWORK=off make dev
bash scripts/ci.sh test
bash scripts/ci.sh fuzz
bash scripts/ci.sh security
npm ci
npx --no-install playwright install chromium
npm run test:browser
```

The adapter also requires `npm run prepare:browser` before browser tests. The core offers `npm run test:ui` for shared UI component tests. `CI_ARTIFACT_DIR` and `PLAYWRIGHT_OUTPUT_DIR` may select evidence directories; defaults use temporary storage. `CI_FUZZ_TIME` overrides the default 30 seconds per target.

`test` runs ordinary tests, race tests, vet, build, module verification and explicit named boundary tests. It rejects an empty named test selection. The core checks a native OpenAPI 3.2 fixture and the bundled asset checksum/license manifest. The adapter generates, checks and exports its actual example document.

The adapter's committed basic example uses a Darwin/arm64 generation profile. The macOS job checks freshness before regeneration. Other platforms generate for their own actual profile and then verify that profile. A platform-specific fingerprint change is not a substitute for the canonical freshness check.

Browser tests compile an isolated fixture, own an ephemeral loopback port, block unexpected network origins, require rendered documentation, and reject console/runtime errors. They check safe submission defaults, explicitly enabled submission with a Bearer token, readable component names, enum descriptions, definition selection and mobile rendering. Screenshots and failure traces are retained outside source. These focused flows do not claim that every OpenAPI 3.2 object has full Swagger UI rendering support.

## Public module access and permissions

Default CI uses the public Go proxy and checksum database with `GOPRIVATE`, `GONOPROXY` and `GONOSUMDB` empty. Checkout credentials are not persisted. Fork pull requests require no secrets; `OPENAPI_READ_TOKEN` is no longer required. The optional path-scoped credential helper and its fake-value boundary tests remain available for separately configured private mirrors; they are not enabled by public CI.

Only preparation, promotion and release jobs receive the minimum write permissions for their own repository. They never execute fork PR code through `pull_request_target`. See [release automation](../CONTRIBUTING.md).

## Published module consumption

The `remote` jobs run for published branch commits on push or manual dispatch. They resolve the actual full commit SHA to its remote Go module version, install the CLI at that fixed version, create a separate consumer outside the checkout, and verify that no selected module is replaced. The adapter also checks first generation, repeated generation, unchanged business source, runtime export and a stripped executable. Ordinary pull requests run source validation without treating an unpublished merge commit as a published module.

To verify an already published commit locally:

```sh
CI_COMMIT_SHA=<full-published-commit-sha> bash scripts/ci.sh remote
```

Successful local runs do not establish that GitHub jobs passed. Inspect the actual run and uploaded evidence. Artifacts are retained for 14 days; failures in environment provisioning or authorization remain failures, not skipped acceptance gates.

## Tagged release acceptance

`Release policy` tests real isolated Git histories and rejects invalid versions, incompatible merge directions, moved/lightweight tags, local replacements, unstable core dependencies, missing CI gates and edits to published releases. `Required checks` aggregates every applicable gate; only unpublished PR merge commits may skip remote consumption.

The release workflow repeats independent consumption on Linux and macOS with `CI_VERSION` set to the exact tag and the public proxy as the only source. It checks the module origin against the expected full SHA, keeping checksum verification enabled and caches empty. The downloaded CLI assets are separately executed on Linux/amd64, macOS/arm64 and Windows/amd64. This Windows smoke check validates the version and document checker, not the full library matrix.

```sh
GOPROXY=https://proxy.golang.org CI_VERSION=v0.0.1 CI_COMMIT_SHA=<tag-commit-sha> bash scripts/ci.sh remote
```
