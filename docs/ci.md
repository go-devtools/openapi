# Independent CI

Each repository checks out only its own source. `GOWORK=off` and `GOTOOLCHAIN=local` prevent a local workspace or automatic toolchain download from masking module requirements. No `replace` directive is permitted in the manifest or selected module graph.

## Toolchain

The workflow pins Go **1.27.1**, Gin **v1.12.0** (adapter only), Node **24.20.0**, npm **11.19.0**, Playwright **1.63.0**, and govulncheck **v1.7.0**. Official Actions use full commit SHAs. Node packages use the committed npm lockfile. Browser binaries come from the pinned Playwright distribution.

The runtime matrix executes on `ubuntu-24.04` (amd64) and `macos-15` (arm64). It does not claim Windows runtime verification. Go 1.27.1 and Gin v1.12.0 were also the latest stable versions when this matrix was configured; no newer combination is claimed. A future version must be added explicitly and actually tested.

Every job starts with separate module and build cache paths under runner temporary storage. Workflow jobs do not restore a shared Go cache. Evidence records actual `GOOS`, `GOARCH`, toolchain, CGO and module versions. Security checking fails on reachable vulnerabilities; module-only advisories are still printed and must not be described as absent.

## Local commands

Use the pinned tools and configure read access to any private modules first:

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

## Private module credentials

The default GitHub Actions token reads only the current repository. While the core is private, the adapter repository needs a repository Actions secret named **`OPENAPI_READ_TOKEN`**, containing a fine-grained token with **Contents: read** for **`openapi-golang/openapi` only**. Organization policy may additionally require owner approval. Create and enter credentials through GitHub's trusted UI; never commit them or put them in module URLs.

The workflow disables persisted checkout credentials. Its process-scoped Git credential helper returns credentials only for exact HTTPS paths under the two product repositories. The current repository uses its own Actions token; the adapter uses the separate read token only for the core. Unrelated hosts, repositories, nested paths and store/erase operations receive no credentials. Tests exercise the boundary using fake values. The helper never changes global Git configuration or stores credentials.

The adapter fails explicitly when the required secret is absent. Untrusted fork pull requests do not receive repository secrets; this workflow does not use `pull_request_target` to work around that boundary. Do not upload personal SSH keys or broaden repository visibility to make CI pass.

## Published module consumption

The `remote` jobs run only for published `main` commits on push or manual dispatch. They resolve the actual full commit SHA to its remote Go module version, install the CLI at that fixed version, create a separate consumer outside the checkout, and verify that no selected module is replaced. The adapter also checks first generation, repeated generation, unchanged business source, runtime export and a stripped executable. Ordinary pull requests run source validation without treating an unpublished merge commit as a published module.

To verify an already published commit locally:

```sh
CI_COMMIT_SHA=<full-published-commit-sha> bash scripts/ci.sh remote
```

Successful local runs do not establish that GitHub jobs passed. Inspect the actual run and uploaded evidence. Artifacts are retained for 14 days; failures in environment provisioning or authorization remain failures, not skipped acceptance gates.
