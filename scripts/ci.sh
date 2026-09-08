#!/usr/bin/env bash
# Run reproducible module checks without changing global tool or credential configuration.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
export GOWORK=off GOTOOLCHAIN=local GIT_TERMINAL_PROMPT=0
# 公开模块使用代理与校验数据库，不依赖仓库凭据。
# Public modules use the proxy and checksum database without repository credentials.
export GOPRIVATE= GONOPROXY= GONOSUMDB= GOSUMDB=sum.golang.org
ci_module="$(go list -m)"
ci_artifacts="${CI_ARTIFACT_DIR:-}"
if [[ -z "$ci_artifacts" ]]; then
  ci_artifacts="$(mktemp -d "${TMPDIR:-/tmp}/openapi-ci-artifacts.XXXXXX")"
fi
mkdir -p "$ci_artifacts"
[[ "$(go env GOVERSION)" == go1.27.1 ]] || { echo 'ci.toolchain: Go 1.27.1 is required'; exit 2; }

# Verify that required named gates exist before executing their real tests.
required_tests() {
  local ci_package="$1" ci_names="$2" ci_available ci_name
  local ci_expected=()
  ci_available="$(go test "$ci_package" -list "^($ci_names)$")"
  IFS='|' read -r -a ci_expected <<< "$ci_names"
  for ci_name in "${ci_expected[@]}"; do
    grep -Fxq "$ci_name" <<< "$ci_available" || { echo "ci.test.missing: $ci_name"; exit 2; }
  done
  go test "$ci_package" -run "^($ci_names)$" -count=1 -v | tee "$ci_artifacts/boundaries.txt"
}

# Exercise one bounded fuzz target and reject empty target selection.
fuzz_target() {
  local ci_package="$1" ci_name="$2" ci_available
  ci_available="$(go test "$ci_package" -list "^$ci_name$")"
  grep -Fxq "$ci_name" <<< "$ci_available"
  go test "$ci_package" -run '^$' -fuzz "^$ci_name$" -fuzztime="${CI_FUZZ_TIME:-30s}" | tee "$ci_artifacts/$ci_name.txt"
}

case "${1:-test}" in
  test)
    go env -json GOVERSION GOOS GOARCH CGO_ENABLED GOFLAGS GOWORK GOTOOLCHAIN | tee "$ci_artifacts/environment.json"
    go mod download
    go mod verify | tee "$ci_artifacts/modules.txt"
    go list -m -json all > "$ci_artifacts/module-graph.json"
    if [[ "$ci_module" == github.com/go-devtools/gin-swagger ]]; then
      [[ "$(go list -m -f '{{.Version}}' github.com/gin-gonic/gin)" == v1.12.0 ]] || { echo 'ci.gin.version: Gin v1.12.0 is required'; exit 2; }
      # The committed example uses the documented Darwin/arm64 generation profile.
      if [[ "$(go env GOOS)/$(go env GOARCH)" == darwin/arm64 ]]; then
        go run ./cmd/gin-swagger check --dir ./examples/basic --output ./internal/apidoc | tee "$ci_artifacts/committed-freshness.json"
      else
        echo 'Committed-generation freshness is checked by the Darwin/arm64 job; this job generates for its actual runtime profile.' | tee "$ci_artifacts/generation-profile.txt"
      fi
      go run ./cmd/gin-swagger generate --dir ./examples/basic --output ./internal/apidoc
      go run ./cmd/gin-swagger check --dir ./examples/basic --output ./internal/apidoc
    fi
    go test -count=1 -json ./... | tee "$ci_artifacts/tests.json"
    go test -race -count=1 -json ./... | tee "$ci_artifacts/race.json"
    go vet ./... 2>&1 | tee "$ci_artifacts/vet.txt"
    go build ./...
    if [[ "$ci_module" == github.com/go-devtools/openapi ]]; then
      required_tests ./internal/verify 'TestNoFrameworkDependencies|TestRuntimeDependencyBoundary|TestNoLocalReplace|TestExternalFrontend|TestCICredentialScope'
      go run ./cmd/openapi check --spec ./testdata/golden/openapi32-full.json
      go test ./swaggerui -run '^TestBundledAssetLicenses$' -count=1 -v | tee "$ci_artifacts/asset-licenses.txt"
    else
      required_tests ./internal/verify 'TestRuntimeDependencyBoundary|TestNoLocalReplace|TestGeneratorBootstrap|TestCICredentialScope'
      go run ./examples/basic --export "$ci_artifacts/openapi.json"
      go run ./cmd/gin-swagger check --spec "$ci_artifacts/openapi.json"
    fi
    ;;
  fuzz)
    if [[ "$ci_module" == github.com/go-devtools/openapi ]]; then
      fuzz_target ./internal/comment FuzzDirective
      fuzz_target ./internal/validate FuzzReferenceGraph
      fuzz_target ./internal/validate FuzzSchemaNumberTraits
      fuzz_target ./compiler FuzzStandaloneComponentIdentity
    else
      fuzz_target ./internal/routes FuzzRoutePath
    fi
    ;;
  security)
    ci_tools="$(mktemp -d "${TMPDIR:-/tmp}/openapi-ci-tools.XXXXXX")"
    trap 'rm -rf "$ci_tools"' EXIT
    GOBIN="$ci_tools" go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
    "$ci_tools/govulncheck" -show verbose ./... | tee "$ci_artifacts/vulnerabilities.txt"
    ;;
  remote)
    [[ "${CI_COMMIT_SHA:-}" =~ ^[a-f0-9]{40}$ ]] || { echo 'ci.remote.commit: an actual published full commit SHA is required'; exit 2; }
    ci_version="$(go list -m -f '{{.Version}}' "$ci_module@${CI_VERSION:-$CI_COMMIT_SHA}")"
    if [[ -n "${CI_VERSION:-}" ]]; then
      [[ "$ci_version" == "$CI_VERSION" ]] || { echo 'ci.remote.version: requested tag was not selected'; exit 2; }
    fi
    go mod download -json "$ci_module@$ci_version" > "$ci_artifacts/module-origin.json"
    grep -Fq "\"Hash\": \"$CI_COMMIT_SHA\"" "$ci_artifacts/module-origin.json" || { echo 'ci.remote.origin: selected version does not match the expected commit'; exit 2; }
    [[ -n "$ci_version" ]] || exit 2
    ci_consumer="$(mktemp -d "${TMPDIR:-/tmp}/openapi-ci-consumer.XXXXXX")"
    trap 'rm -rf "$ci_consumer"' EXIT
    cp -R internal/verify/testdata/external/. "$ci_consumer/"
    cp go.sum "$ci_consumer/go.sum"
    if [[ "$ci_module" == github.com/go-devtools/openapi ]]; then
      printf 'module example.test/consumer\n\ngo 1.27.1\n\nrequire %s %s\n' "$ci_module" "$ci_version" > "$ci_consumer/go.mod"
      ci_cli=openapi
    else
      sed 's|module github.com/go-devtools/gin-swagger|module example.test/gin-consumer|' go.mod > "$ci_consumer/go.mod"
      ci_cli=gin-swagger
    fi
    export GOBIN="$ci_consumer/bin"
    go install "$ci_module/cmd/$ci_cli@$ci_version"
    "$GOBIN/$ci_cli" version | tee "$ci_artifacts/installed-cli.json"
    grep -Fq "$ci_version" "$ci_artifacts/installed-cli.json"
    cd "$ci_consumer"
    go mod edit "-require=$ci_module@$ci_version"
    if [[ "$ci_cli" == openapi ]]; then
      go mod tidy
      "$GOBIN/openapi" schema --dir . --type Request --projection request --output "$ci_artifacts/request.schema.json"
    else
      go mod download
      ci_before="$(find . -type f -name '*.go' -exec shasum -a 256 {} \; | LC_ALL=C sort)"
      "$GOBIN/gin-swagger" generate --dir . --timeout=2m
      "$GOBIN/gin-swagger" check --dir . --timeout=2m
      ci_generated="$(shasum -a 256 internal/apidoc/zz_openapi.gen.go)"
      "$GOBIN/gin-swagger" generate --dir . --timeout=2m
      [[ "$ci_generated" == "$(shasum -a 256 internal/apidoc/zz_openapi.gen.go)" ]]
      [[ "$ci_before" == "$(find . -type f -name '*.go' ! -path './internal/apidoc/*' -exec shasum -a 256 {} \; | LC_ALL=C sort)" ]]
    fi
    [[ -z "$(go list -m -f '{{if .Replace}}{{.Path}}{{end}}' all)" ]] || { echo 'ci.remote.replace: the consumer must not use replacements'; exit 2; }
    [[ "$(go list -m -f '{{.Version}}' "$ci_module")" == "$ci_version" ]]
    go test -race -count=1 -json ./... | tee "$ci_artifacts/remote-tests.json"
    go mod verify
    if [[ "$ci_cli" == gin-swagger ]]; then
      go build -trimpath -ldflags='-s -w' -o "$ci_consumer/application" .
      "$ci_consumer/application" "$ci_artifacts/remote-openapi.json"
      "$GOBIN/gin-swagger" check --spec "$ci_artifacts/remote-openapi.json"
    fi
    ;;
  *) echo 'Usage: scripts/ci.sh {test|fuzz|security|remote}'; exit 2 ;;
esac
