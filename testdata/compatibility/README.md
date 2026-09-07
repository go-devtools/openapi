# Published Bundle writer fixtures

These are actual format-1 outputs from the public compiler SDK, not manually composed OpenAPI JSON. [Source types](source/api/user.go) define a tag-free `(Request) (Response, error)` handler. The [generator](source/generate/main.go) registers a framework-neutral return-value frontend and compiles that source without executing the handler.

| Fixture | Published writer | SHA-256 of Bundle bytes |
| --- | --- | --- |
| `writer-72140a9.bundle.json` | `github.com/openapi-golang/openapi v0.0.0-20260907071604-72140a9a7490` | `9899cf64e97d2bcd4cc414cbda8bdc6514593605d2fd188aa0537a473bd8aba3` |
| `writer-3654588.bundle.json` | `github.com/openapi-golang/openapi v0.0.0-20260907074029-365458867d54` | `06e40fb5bc47d90e0fc4f6d9bbe261abbdd4b709998ffeaee2b29a64535a9404` |

Both were produced with Go 1.27.1 on Darwin/arm64, CGO enabled, default build settings, `GOWORK=off`, and no local replacement. The writer versions were loaded through their real module archives. The generated profile's compiler identity is `openapi/compiler-inputs-v3`; its frontend is `compatibility-return-v1`. It records the source module's selected inputs, so changing its dependency version changes its module digest/fingerprint even when the resulting wire schema is equivalent.

From the core checkout, reproduce each writer in its own temporary module:

```sh
fixture_source="$PWD/testdata/compatibility/source"
fixture_work="$(mktemp -d)"
mkdir -p "$fixture_work/api" "$fixture_work/cmd/generate"
cp "$fixture_source/api/user.go" "$fixture_work/api/"
cp "$fixture_source/generate/main.go" "$fixture_work/cmd/generate/"
cd "$fixture_work"
GOWORK=off go mod init example.test/bundle-compatibility
GOWORK=off go mod edit -go=1.27.1
GOWORK=off go get github.com/openapi-golang/openapi@v0.0.0-20260907071604-72140a9a7490
GOWORK=off go mod tidy
GOWORK=off go run ./cmd/generate bundle.json
GOWORK=off go list -m -json github.com/openapi-golang/openapi
```

Repeat in a fresh temporary module with the other listed version. Different build targets or effective module inputs legitimately change the profile bytes; do not overwrite archived historical output to make it match the current writer. The tests read these fixtures without fetching modules or regenerating them. The source under `testdata` is excluded from ordinary package discovery and is not another product module.
