# AI-assisted integration

Use [llms.txt](../llms.txt) as the small entry index. Read the guide for the API being used instead of loading generated Bundles or bundled UI JavaScript into the context window. The index follows the [llms.txt proposal](https://llmstxt.org/); it is documentation, not a service endpoint or an external model dependency.

## Choose the package

| Need | Package or command |
| --- | --- |
| Build a document from a generated Bundle and neutral routes | Root openapi package |
| Construct typed native OpenAPI objects with explicit value presence | spec |
| Analyze actual Go source or implement a frontend | compiler |
| Validate actual samples independently | contracttest |
| Serve shared offline UI resources | swaggerui |
| Export a source type or check a document | openapi CLI |

The runtime root must remain independent of compiler, UI, and framework dependencies. External adapters use public compiler views and neutral effects, not internal packages. Check [SDK ownership and budgets](adapter-sdk.md) before implementing a custom callback.

## Reproducible commands

From a checkout using its pinned dependencies:

```sh
GOWORK=off go mod download
GOWORK=off go run ./cmd/openapi version
GOWORK=off go run ./cmd/openapi schema --help
GOWORK=off go run ./cmd/openapi schema --dir ./testdata/types --type Request --projection request
GOWORK=off go run ./cmd/openapi check --spec ./testdata/golden/openapi32-full.json
GOWORK=off make dev
```

The schema command writes JSON to stdout when output is omitted. In an application, point dir at its package and type at the actual Go type expression; generic instances are resolved from loaded types. Use an explicit output path only when a file is wanted. Commands load source statically and do not execute handlers to discover types.

Install the CLI at the same fixed module version used by the application and record its JSON version output. For process exit-code checks, invoke the installed binary: `go run` wraps nonzero program exits and is not the native exit-code interface.

## Consume evidence

| Output | How to use it |
| --- | --- |
| `openapi version` | Record module version, revision, Go toolchain, Bundle format, and OpenAPI version. |
| `openapi schema` | Parse the standalone JSON Schema; do not scrape Swagger UI to recover contracts. |
| `openapi check` | Parse Report diagnostics and check the process exit code. An empty diagnostic list is successful checking. |
| `Document.Report()` | Inspect structured code, severity, source, route, facts, and fix fields. |
| Bundle template diagnostics | Determine whether a selected operation has unresolved behavior before constructing its document. |

Successful commands exit 0. Document validation errors exit 1. Flag syntax errors exit 2; subcommand help exits 0. Setup failures emit a JSON envelope on stderr with code, severity, and message. Flag usage text is human-readable stderr, so stdout and stderr should be captured separately. Do not infer success from a file left by an earlier invocation.

Diagnostic codes are the integration key; English messages explain the specific instance. Source.Kind distinguishes derived, declared, and unresolved provenance where present. A declared constraint describes a contract and does not prove that application code enforces it.

## Resolve a diagnostic

1. Read the selected route, source location, diagnostic code, and available facts.
2. Compare the actual Go type, codec, and build conditions with the declaration.
3. Correct an inaccurate semantic comment or add one centralized public extension for an unsupported codec or helper. Do not add DTO tags, change business handlers, or fabricate a default response to suppress the error.
4. Regenerate with matching build inputs and configuration. Run freshness checking and validate actual positive and negative samples with contracttest.

For example, openapi.comment.type on minimum for a string is a declaration/type mismatch. Determine the intended constraint from the application before replacing it with minLength. openapi.generate.stale means source or generation inputs changed; regenerate rather than editing zz_openapi.gen.go.

For explicit request/response contracts, read [request and response declarations](request-response-declarations.md). Use TypeIn for an explicit package context; a declaration cannot erase an unknown-effect diagnostic.

See the [support matrix](openapi32-matrix.md) for limits. Automatic derivation, explicit declaration, centralized adaptation, and unresolved behavior are distinct outcomes. Budget errors require a bounded rule or an explicitly justified budget change; they do not authorize partial contracts.

Imported DTOs retain the metadata of their source declarations without becoming operation candidates. For an imported annotation error, inspect its logical package/file location and the selected Route field; Facts retain the application call or explicit declaration that used it. See [imported metadata](imported-metadata.md). Generated reports use physical source positions from the loaded snapshot, including overlays, rather than a later filesystem read.

For a specific field or response, use the public [source explanation API](explain.md). Preserve `implementation: "not-proven"` when reporting declarations, inspect original diagnostic codes and fixes, and do not treat successful explanation output as successful runtime route validation.
