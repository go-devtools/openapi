# Parameter objects and explicit wire types

The compiler can project annotated Go input objects into named parameters without imposing a framework's field rules. Adapters supply `WireCodec` and emit `Effect{Kind: ParameterObject, In: ..., Payload: ..., Codec: ...}`. The shared compiler retains annotations, enum handling, component identity, required properties, and parameter conflict diagnostics. `Style` and `Explode` come from the frontend; the core does not infer framework serialization.

`ParameterObject` accepts a finite object projection for query, path, header, or cookie parameters. It resolves generated component references and expands known properties. Dynamic objects, incompatible duplicate projections, unresolved root references, and conditional object combinations return diagnostics. Path parameters are required by the OpenAPI contract. A required object property becomes a required parameter; this does not imply that business code enforces its declaration. Body-level required and property-level required remain separate.

## Optional type projection

A codec may also implement the public `WireTypeCodec` interface:

```go
type WireTypeCodec interface {
    ProjectType(ProjectionRequest, func(types.Type) (*spec.Schema, error)) (*spec.Schema, bool, error)
}
```

The callback receives a read-only request for the current actual Go type. It returns a schema with `handled=true`, or delegates to the shared structural projector with `handled=false`. Returning `handled=true` without a schema is an error. Caller-provided TypeMapper rules run first, enabling centralized application overrides.

The recursive function preserves the current projection's type budget, component references, annotations, and codec identity. Invoke it synchronously during the callback; do not retain it, invoke it concurrently, or call business methods. A recursive callback consumes the same type budget and returns a diagnostic on exhaustion. Each Project.Schema call owns separate state and can be invoked concurrently when the supplied codec and mapper values also support concurrent reads.

Implementing this interface makes the codec responsible for its standard named types and custom method semantics. The core no longer assumes that JSON methods, json.Number, RawMessage, time.Time, or time.Duration define that codec's wire representation. Delegation retains shared structural handling, named components, and annotations. A text codec should explicitly handle pointer presence and byte collections if JSON null/Base64 behavior does not apply. A field-only WireCodec retains the previous default type behavior for compatibility.

Returned handled schemas are copied before use so later annotation or consumer edits cannot change a codec-owned schema. Codec names must be stable and include every configuration choice that changes structure. Actual Go type identity, direction, media type, and codec identity distinguish components.

## Resolved symbols

`Value.Object` retains a standard go/types symbol through supported identifier, selector, and assignment propagation. It is lexical evidence for a frontend's explicitly recognized constants or exported configuration objects, not proof about arbitrary runtime mutation. No types.Object, AST, callback, or codec is serialized into Bundle; only the resulting schemas, parameters, source facts, and diagnostics are stored.

## Evidence and remaining boundaries

Core tests use a neutral text protocol and actual Go source. An independent public-SDK consumer verifies parameter projection without importing an HTTP framework or core internals. Decoded byte lists remain arrays rather than Base64 strings; optional pointer parameters cannot invent JSON null. Source annotations and required flags survive expansion. Callback errors, recursion budgets, and returned-schema ownership are also checked.

Finite method/media selection tables, complete helper summaries, parameter representation conflicts between raw and decoded reads, whole-object conditional constraints, and complete source-provenance coverage remain part of the broader goal. Unsupported cases must remain explicit diagnostics rather than guessed parameter definitions.
