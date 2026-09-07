# Public adapter SDK

The public API separates source compilation from runtime document construction. Pin a module version when integrating this pre-1.0 SDK.

## Runtime protocol

`openapi.NewBundle(BundleData) (Bundle, error)` creates an immutable format-1, OpenAPI-3.2.0 snapshot. `ParseBundle` validates the interchange format. `GeneratedBundle` supports static generated factories and defers parsing failures to `Validate` or `Build` without panicking.

`Bundle.Index()` and `Snapshot()` return independent copies. `OperationKey` identifies a source template; the default operationId is derived from the normalized HTTP method and path. `Route` already contains a standard path. Framework path syntax belongs to the adapter.

`Build(Bundle, []Route, Config) (*Document, error)` links contracts without registering routes or reading source. `Document.JSON()` and `Report()` return copies; `WriteFile` replaces output atomically. A Bundle can be used concurrently by independent document instances.

## Offline checking

`Check` uses default settings. `CheckWithOptions` accepts `BaseURI`, preloaded `Resources`, and raw `ExampleResources`. `Config.Validation` uses the same settings for component pruning and final checking during Build. Inputs are read only during the call; the checker does not load files or retrieve URLs. See [offline references](references.md) for resource scopes, budgets, and rendering boundaries.

## Compilation protocol

`compiler.Load(context.Context, LoadOptions) (*Project, error)` loads Go syntax and types under actual build conditions. Public views expose `go/ast`, `go/token`, `go/types`, and `go/constant`; they do not expose third-party SSA. The Project owns its views, which callers must treat as read-only. `Package.Sizes` describes actual target type sizes.

`Project.Type` resolves loaded types and generic instances. `Project.TypeIn` adds an explicit source-package context. `Project.Packages` and `Functions()` remain the requested source roots and their function candidates; importing DTO metadata does not add dependency functions to operation discovery. `Project.Schema` creates an independent projection with its own recursive cache. `Projection.Standalone()` exports the reachable component closure as `$defs`; `StandaloneWithOptions` adds explicit resource identities, offline dependencies, dialect fallback, and budgets. Direction, media type, codec, and generic identity distinguish projections. Read-only projections may run concurrently after loading. See [standalone Schema](standalone-schema.md).

`Frontend` registers explicit Go callbacks: `Name`, `Match`, `Entry`, `Callback`, `Call`, `CallOutcomes`, `Return`, and `CarriesEffects`. Adapters describe framework behavior as neutral effects. The core schedules value propagation, statement order, branches, returns, and bounded helper calls. Return callbacks receive ordinary Go values and do not require an HTTP context.

`compiler.Compile(context.Context, Options) (*Result, error)` composes loading, comments, projection, and Bundle generation. `Result.Write` atomically updates the owned `zz_openapi.gen.go`; the generated factory imports only the lightweight runtime. It does not import or execute business handlers. `Result.Check` regenerates bytes and compares the complete output without writing.

## Failure and ownership rules

Default analysis limits are 2,048 packages, 4,096 projected type nodes, 12 helper levels, 128 paths, 10,000 calls, and 32 callback iterations. Comments are limited to 64 KiB. Document input defaults to 8 MiB, 128 levels, and 200,000 JSON nodes. Budget exhaustion produces a diagnostic; truncated analysis is not a complete contract.

Template diagnostics travel with the Bundle and block Build when their route is selected. Global load failures, annotation syntax errors in explicitly selected source roots, and frontend conflicts block compilation. Malformed metadata in an imported dependency is preserved and reported when the affected type, field, or closed-enum constant is projected; it does not poison unrelated DTOs. Extension callbacks must be deterministic and must not execute business functions or derive output from the clock, network, or machine-specific paths.

Format-1 readers accept the declared capabilities `oas32`, `schema2020-12`, and `request-conditions-v1`; unknown required capabilities and future formats are rejected. Public consumers in `internal/verify/testdata/external` exercise the SDK from a separate Go module. `OPENAPI_TEST_CORE_VERSION` selects a fixed remote version for that boundary test; the development default uses an isolated local replacement.

## Version compatibility

Keep three compatibility decisions separate:

| Boundary | Supported contract | Failure behavior |
| --- | --- | --- |
| Go source SDK | This pre-1.0 SDK is selected by an exact module version. No compatibility with every older or newer Go API is promised. Compile custom frontends against the selected core; an adapter's `go.mod` records its required core version. | Go compilation reports missing or changed public APIs. Re-run external consumer and adapter checks before upgrading. |
| Bundle writer/reader | Reader and writer format **1**, OpenAPI **3.2.0**, and the required capabilities `oas32`, `schema2020-12`, `request-conditions-v1`. A writer must declare every capability its payload needs. | Future formats, different specification versions, unknown required capabilities and unknown protocol fields are rejected. `GeneratedBundle` defers that error to `Validate`/`Build`; it never fabricates an empty successful contract. |
| Executable build profile | Source-generation conditions and selected modules are recorded separately from the Bundle format. Gin validates them by default; core callers opt in with `Config.VerifyRuntimeBuild`. | Known target/module/configuration mismatches fail; unavailable metadata produces diagnostics. Format compatibility does not prove source freshness or executable equivalence. |

`Profile.Generator` and `Profile.Frontend` identify the producing algorithms. Readers do not require those strings to equal their own versions. They are not substitutes for a format or capability declaration. Adding behavior that an older reader cannot preserve requires a new capability or format and an explicit compatibility test; retaining format 1 alone is not permission to silently change its meaning.

`TestPublishedBundleCompatibility` reads archived output generated from real tag-free Go source with two published core versions: `v0.0.0-20260907071604-72140a9a7490` and `v0.0.0-20260907074029-365458867d54`. The current reader links each into a document and independently validates both valid and invalid request/response samples. `TestSerializedBundleCompatibilityRejection` exercises serialized format, specification, capability and unknown-field failures. The [fixture provenance and reproduction commands](../testdata/compatibility/README.md) identify the exact writers and input source.

This verifies those writer/reader combinations for the archived ordinary JSON contract; it does not promise every historical module pair, every optional capability, or future source API compatibility. The ordinary and remote CI jobs run current consumer code against the exact selected module, and separately exercise finite request conditions. Upgrade an adapter's fixed core dependency only after the core tests pass, then regenerate and check the adapter with workspace disabled.

## Extension entry points

| Behavior | Public entry point | Guide |
| --- | --- | --- |
| Parameter objects and non-JSON representations | `ParameterObject`, `WireCodec`, `WireTypeCodec`, `Value.Object` | [Parameter codecs](parameter-codec.md) |
| Correlated return values and effects | `CallOutcomes`, `CallOutcome` | [Call outcomes](call-outcomes.md) |
| Method and media-dependent contracts | `RequestCondition`, conditional variants | [Request conditions](request-conditions.md) |
| Individual body fields and encoding | `RequestField`, `WireSchema`, `Encoding` | [Request fields](request-fields.md) |
| Headers, status commitment, and body framing | `ResponseHeader`, `ResponseCommit`, `ResponseItem` | [Response effects](response-effects.md) |
| Inner payload projection and compile-time wrapping | `PayloadMediaType`, `TransformSchema` | [Response effects](response-effects.md) |
| Captured function values and synchronous callbacks | `CallbackPlan`, `CallbackRepeat`, `MaxIterations` | [Callbacks](callbacks.md) |

A handled custom codec owns its wire representation; it does not inherit JSON assumptions. Recursive projection callbacks are synchronous and must not be retained or called concurrently. Schema callbacks receive detached data and return values that are copied before storage. Unknown calls carrying effect objects require an explicit rule. Arbitrary heap aliasing, asynchronous effects, and unsupported control flow remain diagnostic boundaries.

Imported type comments, constraints, enum labels, generic field origins, immutable loaded views, and physical source coordinates are described in [imported metadata](imported-metadata.md). Projection reports preserve original declaration locations and calling evidence; runtime linking attaches the selected method/path to template diagnostics.

`Options.Explain` enables compile-time evidence capture. `Result.Explain(ExplainQuery)` returns detached field/type/handler/response provenance without changing the runtime Bundle or rerunning extension rules. See [source explanations](explain.md) for query semantics, ownership, and budgets.
