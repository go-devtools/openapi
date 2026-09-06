# Request and response declarations

Ordinary function comments can describe a client contract when it is not present in inferred effects. The core resolves real Go types and uses the same Schema projection as registered frontends. Framework adapters do not parse this syntax or own its type resolution.

```go
// Create a user.
// @openapi request mediaType="application/json" type="CreateUserRequest" required
// @openapi response status=201 mediaType="application/json" type="User"
// @openapi response status="default" mediaType="application/json" type="example.com/service/errors.Problem"
func Create(/* existing parameters */) { /* existing implementation */ }
```

The snippets show documentation placement; keep application handlers, route registration, and DTO tags unchanged when integrating documentation. A declaration records an intended contract, not evidence that a handler enforces it. Unknown calls, dynamic status codes, unmodeled codecs, and other analysis errors remain diagnostics. A default declaration never converts an unknown status into a known default response.

## Type names and loading

Unqualified names resolve in the handler's package. Fully qualified paths refer to packages already in the loaded project, including its existing dependencies. For a DTO package that the handler does not import, include it in `LoadOptions.Patterns` or use `./...`; comments do not trigger a second package load, dependency download, `go.mod` edit, or unused import.

Supported expressions include `Request`, `*Request`, `[]Request`, and `example.com/service/dto.Envelope[example.com/service/users.User]`. Each generic must be fully instantiated and its type arguments must satisfy actual Go constraints. Foreign unexported types cannot be referenced. An expression is limited to 8,192 bytes.

Centralized generation entry points can use `Project.TypeIn(packagePath, expression)` to resolve types in an explicit package context. `Project.Type(expression)` accepts a short name only when all successful root-package resolutions identify the same Go type. It reports `openapi.type.ambiguous` for different same-named types. Resolution preserves the original `go/types` identity and does not mutate loaded package scopes.

Imported DTO packages retain their type, field, and constant comments automatically, including fields of instantiated generic types. Packages mentioned only in comments still need an explicit source-root pattern because annotations do not initiate package loading. See [imported metadata and source evidence](imported-metadata.md).

## Accepted keys

| Declaration | Required keys | Optional keys |
| --- | --- | --- |
| Request payload | `mediaType`, `type` | Boolean `required` |
| Response payload | `status`, `mediaType`, `type` | None |
| Bodyless response | `status` | None |

Statuses are exact integers from 200 through 599 or the string `"default"`. Status 204 and 304 declarations cannot contain a body. Informational responses and status ranges need an explicit centralized model. Media types must be concrete lowercase types without parameters or wildcards.

`type="any"` explicitly declares unconstrained JSON and remains distinct from an unresolved payload. Omitting a request type is an error. Bodyless responses can be documented as `@openapi response status=204`.

## Merging with derived behavior

Declarations add absent request media and response alternatives without deleting observed responses, headers, or encodings. For a matching request media or response status/media, the projected wire schema must equal the observed schema. A conflicting type or body-presence declaration produces `openapi.declaration.conflict`; it does not silently union incompatible types or overwrite source facts.

This comparison is conservative. Different named types, overlapping unions, wrapped renderers, or semantically equivalent but differently structured schemas may require a centralized frontend rule. General Schema equivalence is not inferred. A declared alternative is supplemental: an explicitly documented status may coexist with different statuses derived from the handler.

Body-level `required=true` is a semantic client constraint. It does not prove empty-body rejection. It cannot be reversed by a conflicting explicit optional declaration. Existing field-level constraints remain separate. Repeated declarations must agree on presence.

When a matching effect has an explicit codec, declaration projection reuses that codec. Conflicting codecs are diagnosed. Otherwise only the core's standard JSON projection is selected automatically; other representations need a centralized codec rule. Type mappers still require named `Options.Configuration` inputs for freshness.

## Diagnostics and provenance

| Code | Meaning |
| --- | --- |
| `openapi.declaration.invalid` | Missing, misplaced, or incorrectly typed declaration keys. |
| `openapi.declaration.type` | A complete actual type cannot be resolved in the loaded scope. |
| `openapi.declaration.schema` | Type projection or matching codec selection failed. |
| `openapi.declaration.conflict` | Declaration disagrees with an existing contract or explicit presence declaration. |

Successful declarations retain `Source.Kind="declared"`, the `openapi.comment.request` or `openapi.comment.response` rule, symbol, and comment location. Finite method/media variants also preserve their request condition. Invalid candidate declarations do not block unrelated selected routes; selecting the invalid candidate fails. Malformed directive syntax remains a source-loading error.

Regression tests compile real source, compare repeated generated Bundles, validate positive and negative JSON samples independently, inspect provenance, and verify that business source and module files remain unchanged. The public SDK consumer exercises the same behavior from a separate module. See [request conditions](request-conditions.md), [request fields](request-fields.md), and [adapter SDK](adapter-sdk.md) for central extension APIs.
