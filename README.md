# openapi

[简体中文](README.zh-cn.md)

Framework-independent Go source contract compilation and native OpenAPI 3.2 tooling.

This repository is under active implementation. The full acceptance target is recorded in [GOAL.md](GOAL.md); current evidence and remaining work are tracked in [status](docs/status.md) and [verification](docs/verification.md). It is not yet a completed or released product.

## Requirements

- Exactly Go 1.27.1 for minimum-version acceptance.
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

## License

New project code is licensed under [MIT](LICENSE). Third-party assets retain their original licenses and notices.
