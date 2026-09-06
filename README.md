# openapi

[Simplified Chinese](README.zh-cn.md)

Framework-independent Go source contract compilation and native OpenAPI 3.2 tooling.

This pre-1.0 SDK evolves between pinned versions. Use the public APIs and check the documented capability boundaries before depending on advanced behavior.

## Requirements

- Go 1.27.1 for development and verification.
- No Gin, Fiber, or Echo dependency in the core or its tests.

## Architecture

Go source and real codec behavior define structure; ordinary comments provide business meaning. Documentation generation must not change business DTO tags, handler bodies, signatures, or existing route registration.

The core owns type projection, comments, neutral effects, Bundle and OpenAPI models. Framework adapters own framework call semantics, route syntax, handler evidence and mounting. Runtime document construction never imports the compiler or reads application source.

## Capability boundaries

- **Automatically derived:** types, supported wire representations and recognized source effects, as verified by implementation tests.
- **Explicitly declared:** semantic constraints and advanced contracts; these are not proof that the server enforces them.
- **Centrally adapted:** custom codecs and unsupported project helpers through explicit Go extension points.
- **Unresolved:** ambiguous or unsupported behavior must produce a diagnostic rather than a guessed response.

Future Fiber and Echo adapters are extension directions only. They are not products delivered or claimed as supported by this repository.

## Offline reference checking

`CheckWithOptions` accepts an explicit retrieval URI, preloaded OpenAPI / JSON Schema resources, and raw external examples. The same configuration is available as `Config.Validation` during document construction and through `openapi check --resources resources.json`. All inputs are bounded; the checker never fetches URIs. See the [reference API and CLI guide](docs/references.md) for resource scopes, budgets, and current rendering boundaries.

## Schema and source constraints

Raw document checking accepts legal JSON Schemas even when their constraints are inapplicable or unsatisfiable. Source projection separately diagnoses annotation conflicts with inferred wire types and bounds, including named component references. See [schema checks and annotation diagnostics](docs/schema-annotations.md) for the tested behavior and remaining limits.

## Independent contract validation

The optional `contracttest` package accepts explicitly preloaded resources and validates actual JSON samples with an independent engine, including dynamic recursive references. It never fetches missing resources. See the [contract validation guide](docs/contracttest.md) for options, budgets, and remaining boundaries.

## Standalone JSON Schema

`Projection.StandaloneWithOptions` exports one offline document with resource-aware references, embedded explicit dependencies, preserved dialects, and bounded output. The CLI exposes the same options through `openapi schema`. See the [standalone schema guide](docs/standalone-schema.md) for the public SDK, resource rules, and limits.

## Neutral response effects

The public compiler SDK supports commit-time response headers and explicit non-JSON wire schemas. See the [response effect guide](docs/response-effects.md) for ordering, provenance, and limits.

## License

New project code is licensed under [MIT](LICENSE). Third-party assets retain their original licenses and notices.

Project-owned code comments, diagnostics, CLI help, and example text use English. Multilingual encoding tests retain their input values through escaped literals. Upstream assets retain their original form. Commit messages use English; [README.zh-cn.md](README.zh-cn.md) provides corresponding Chinese documentation.

The public compiler supports [parameter object and wire-type codec extensions](docs/parameter-codec.md) without framework dependencies.

Generation frontends can model correlated return values and side effects with [call outcomes](docs/call-outcomes.md).

[Finite request conditions](docs/request-conditions.md) preserve method/media decisions across source generation and runtime linking.

See the [build inputs guide](docs/build-inputs.md) for target profiles, overlays, workspace dependencies, reproducible fingerprints, and custom mapping configuration.

Runtime consumers can call `openapi.CheckRuntimeBuild(profile)` or set `Config.VerifyRuntimeBuild` when linking a document. Known build-condition differences fail; missing metadata is reported. See [build inputs](docs/build-inputs.md) for cross-target exports and verification limits.

The public compiler supports [request-field effects](docs/request-fields.md) for individual body fields, explicit encodings, and composition with whole-body projections. Field presence and body presence remain separate.

The public response SDK also supports sequential `ResponseItem` effects, independent payload codecs, and detached compile-time schema wrapping. See the [response effects guide](docs/response-effects.md) for NDJSON/SSE validation and the remaining framework boundaries.

The public value and response snapshots preserve boxed payload identity and committed headers. Independent stream validation covers protocol-specific line endings, UTF-8 replacement, and bounded input; see the [contract guide](docs/contracttest.md).

Known function values and synchronous callback conventions are analyzed through the public [callback SDK](docs/callbacks.md), with isolated captured cells and bounded repetition. Framework-specific callback behavior remains in adapters.

## AI-assisted integration

Start with [llms.txt](llms.txt) for a compact documentation index and the [AI integration guide](docs/ai-integration.md) for actual commands, structured diagnostics, and public API boundaries. Generated JSON and provenance provide evidence for integration decisions.
