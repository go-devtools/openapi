# Standalone JSON Schema export

`compiler.Project.Schema` projects an actual Go type into a root schema and its component closure. `Projection.Standalone()` exports that projection as a single JSON Schema document. `StandaloneWithOptions` additionally configures resource identity, explicitly preloaded dependencies, and budgets. Neither method runs business handlers or codecs.

## Public SDK

```go
package main

import (
    "context"
    "os"

    "github.com/go-devtools/openapi/compiler"
)

// Export a request contract from an actual type with an explicit portable resource identity.
func main() {
    project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: "."})
    if err != nil {
        panic(err)
    }
    typ, err := project.Type("Request")
    if err != nil {
        panic(err)
    }
    projection, err := project.Schema(compiler.ProjectionRequest{
        Type: typ, Direction: compiler.Input,
    })
    if err != nil {
        panic(err)
    }
    raw, err := projection.StandaloneWithOptions(compiler.StandaloneOptions{
        BaseURI: "https://example.test/schemas/request.json",
        MaxNormalizedBytes: 1 << 20,
    })
    if err != nil {
        panic(err)
    }
    if _, err = os.Stdout.Write(raw); err != nil {
        panic(err)
    }
}
```

The current package must declare `Request`, or the caller must select another loaded type. Use `compiler.Output` for a response projection. The two directions follow their respective wire contracts; exporting a schema does not prove that a server enforces every declared constraint.

The returned bytes do not alias the projection. Concurrent exports may read the same projection and resource map, provided the caller does not modify either input during those calls. Export does not retain or modify caller-owned resource bytes.

## Type alias declarations

Source aliases retain their own descriptions, readable titles, explicit enum arrays, and applicable constraints, including chained and instantiated generic aliases. Each alias declaration is conjoined with its target contract. For example, an alias with `minLength=1` cannot weaken its target's `minLength=3`; adding `maxLength=2` produces a source-located range diagnostic. Constraints on an alias of a named object do not change unrelated uses of that object's component. Recursive aliases use the existing component graph and share the type-expansion budget.

A bare `enum` on an alias produces `openapi.schema.alias-enum`: a Go alias has no distinct constant type from which to infer its own closed value set. Declare `enum=["red","blue"]` explicitly or provide a centralized mapper. Actual named enum types retain their constant-based enumeration behavior. Alias declarations record their source origins when `Explain` is enabled. Explicit `TypeMapper` and `WireTypeCodec` rules continue to take precedence over structural projection and source alias metadata.

These annotations describe the contract at source use sites; they do not create a new Go runtime type or inject validation into an alias. Public tests validate actual JSON values and negative contract examples independently, including generic element substitution and recursive named targets.

## Declared nonnull contracts

`nonnull` excludes JSON null at its declared use site, including references, recursive object pointers, named collections, unconstrained values, RawMessage, and explicitly mapped unions. It does not imply property presence; use `required` separately. Unrestricted uses of the same component and nullable members inside a referenced object keep their original behavior.

For typed schemas, null is removed from the type union. Other schemas receive a null-exclusion constraint while preserving existing `not` and `allOf` constraints. A null-only wire type conflicts with `nonnull`, and `nonnull` cannot be combined with an enabled `nullable` declaration. Constraint applicability is checked against the original wire projection, so excluding null cannot make string-length constraints valid for an integer.

These are declared client/server contracts, not injected decoder validation. The standard decoder can still accept a null that violates a declared contract; independent contract tests detect that mismatch without modifying handlers or DTOs.

## Standard-library wire values

The standard profile maps `time.Time` to a date-time string, `time.Duration` to integer nanoseconds, and `json.Number` to a JSON number. Actual-byte tests include zero timestamps, negative durations, signed and unsigned 64-bit limits, fractional numbers, and numbers beyond binary64's exact integer range. Projection and standalone export preserve the decimal JSON bytes. RawMessage remains an unconstrained JSON value, including objects, arrays, and null.

Signed 64-bit integers use `int64`; unsigned 64-bit integers use `uint64` and retain a nonnegative minimum. The formats follow the OpenAPI [int64](https://spec.openapis.org/registry/format/int64) and [uint64](https://spec.openapis.org/registry/format/uint64) registry entries. Format annotations do not convert numbers into strings or prove that every external validator enforces integer-width bounds.

Byte slices, including aliases and slices of ordinary named uint8 elements, encode as Base64 strings; nil slices permit null. Fixed byte arrays retain their array representation. An effective output method on a named byte element changes a slice into an array of custom element representations and requires the corresponding mapping. Input byte-slice contracts use canonical Base64, which the standard decoder reads without calling element methods. Its alternate array input may call those methods; that additional decoder allowance is not included in the canonical Base64 schema. Use an explicit containing-type mapping or codec if the client contract intentionally requires that array representation.

Scalar declarations on a byte element cannot silently disappear when a slice becomes opaque Base64. Named and alias element declarations produce `openapi.codec.byte-element`; plain documentation alone does not. Map the containing slice to an explicit encoded-content contract or supply a `WireTypeCodec`. Fixed byte arrays expose scalar JSON elements and continue to preserve their element enums. A public SDK test provides a whole-slice Base64 mapping and checks actual valid and invalid enum-byte sequences with an independent validator.

The selected standard compatibility configuration rejects field `format:*` options at runtime, including byte encoding and time/duration formats. Projection reports `openapi.codec.format` rather than silently describing an unencodable response. A different encoder must supply an explicit codec. Options such as `inline` and `unknown` are ignored by this profile and do not flatten ordinary named map fields; tests compare the actual nested objects with their schemas.

## Custom JSON and text value methods

Custom methods are recognized by their exact standard interface signatures, not their names alone. Marshaling requires `MarshalJSON() ([]byte, error)` or `MarshalText() ([]byte, error)`; unmarshaling requires `UnmarshalJSON([]byte) error` or `UnmarshalText([]byte) error`. Byte-slice aliases match, while different named parameter types, variadic parameters, extra parameters, and different result types do not. Tests use real JSON calls to verify that incompatible methods are ignored.

The required Go 1.27.1 standard implementation also calls `AppendText([]byte) ([]byte, error)`, `MarshalJSONTo(*jsontext.Encoder) error`, and `UnmarshalJSONFrom(*jsontext.Decoder) error`. Their signatures and standard-library parameter identities are checked, including type aliases. Streaming JSON interfaces take precedence over legacy JSON interfaces; text appenders precede text marshalers. These custom representations require mappings just like the other effective methods. Recognizing these interfaces does not enable the separate `encoding/json/v2` API or all of its configuration options.

Input and output use their respective methods. An output-only method does not replace ordinary input decoding, and an input-only method does not replace ordinary output encoding. JSON interfaces take precedence over competing text interfaces. Unknown custom wire representations require a direction-specific centralized `TypeMapper`; projection never executes the business method to discover its output.

On a standard JSON field with `,string`, an effective custom JSON or text method owns the representation. Its mapper's object shape, text pattern, and other constraints survive field projection. The compiler does not replace an explicitly mapped object with a string merely because the underlying Go type is an integer. Public tests encode custom objects and labeled text, decode them through the actual input methods, and independently validate their standalone schemas.

Pointer-only output methods can depend on addressability. For example, a named integer field with `,string` may encode as `"7"` when its enclosing struct is passed by value, but as an object when the enclosing struct is passed by pointer. Standalone type projection does not infer all runtime use sites. For this ambiguous combination, it returns `openapi.codec.addressability` rather than selecting one representation from a scalar mapper. Map the containing type to the actual alternatives or supply an explicit `WireTypeCodec`. A public test validates both real outputs against one containing-type mapping and rejects an impossible unquoted integer.

These rules describe the standard `encoding/json` profile. A custom `WireCodec` controls its own field flags, and a `WireTypeCodec` owns its custom type representation. Map keys use their [key-specific receiver rules](#standard-json-map-keys-and-custom-mappings). Custom pointer methods at other use sites still require explicit mappings; this API does not perform arbitrary runtime addressability analysis.

## Standard JSON field names and presence

The standard JSON projection selects actual exported wire fields by embedding depth and explicit tag priority. Same-depth conflicts disappear from both the encoded object and the schema. A named embedded field remains a nested property. Fields promoted through a nil embedded pointer may be absent; direct fields are not made optional merely because another embedding path is optional. Tests compare actual JSON with schema properties for conflicts, tagged precedence, shallow shadowing, repeated paths, unexported embedded structs, and recursive embedding.

Input fields are not automatically required because their Go values are nonpointers. Output presence follows the selected encoder:

| Field option or kind | Output presence |
| --- | --- |
| No omission option | Present, including a nil pointer encoded as null |
| `omitempty` on scalar, pointer, interface, slice, or map | May be absent at an empty Go value |
| `omitempty` on a struct or a nonempty fixed array | Present; the zero Go value alone does not omit it |
| `omitempty` on a zero-length array | May be omitted |
| `omitzero` | May be omitted according to zero-value or `IsZero` behavior |
| Field promoted through a pointer embedding | May be absent when an enclosing pointer is nil |

An interface containing a typed nil pointer is a present interface value and may encode as JSON null even with `omitempty`. A nonnil scalar pointer remains present when it points to zero. The projection does not execute custom `IsZero` methods or prove their implementation; an `omitzero` field is treated as potentially absent.

The `,string` option applies to booleans, numbers, strings, and one pointer layer to those scalar kinds. Nil pointers, including named pointer types, still permit null. The standard compatibility API ignores this option on arrays, slices, byte slices, objects, and RawMessage: their normal JSON representations remain in the schema. Separate codecs remain responsible for their own option semantics.

Ordinary names, Unicode names, and supported punctuation are preserved. A JSON tag name containing backslash or quotation delimiters returns `openapi.codec.fieldname`. Such tags can be truncated or fall back to a Go field name in the selected standard implementation, so the compiler does not claim a portable name from their literal spelling. Supply an explicit `WireCodec` that describes the chosen encoder if these tags are intentional. Projection does not require changing DTOs or handlers.

## Standard JSON map keys and custom mappings

Ordinary string map keys become JSON object property names. Ordinary integer keys become string property names constrained by `^-?[0-9]+$`; map values retain their own schema and a nil map permits JSON null. The numeric-key pattern describes their spelling, not the full decoder range for each integer width.

Custom key text is direction-dependent. Output checks the actual key value's `encoding.TextAppender` and then `encoding.TextMarshaler` method sets; a pointer-only method on a nonpointer map key does not participate. Input checks `encoding.TextUnmarshaler` on a pointer to the key. A method with the same name but a different signature is not that interface. Output-only methods do not affect input projection, and input-only methods do not affect output projection. JSON value methods alone do not determine object-key encoding through the supported `encoding/json` API.

An effective custom text-key method returns `openapi.codec.mapkey` instead of silently assuming numeric keys or accepting arbitrary custom input. Provide a centralized `TypeMapper` for the containing map, or handle it through an explicit `WireTypeCodec`. For example, a map whose keys encode as `key:7` can declare `type: [object, null]`, string `additionalProperties`, and a string `propertyNames` schema with `pattern: ^key:[0-9]+$`. The mapper must describe the actual codec; the compiler never invokes the business method. Test real encoded and decoded payloads with `contracttest`.

These cases are tested with this repository's required Go 1.27.1 toolchain and its default `encoding/json` implementation. In that configuration, named string output keys may also invoke `AppendText` or `MarshalText`. The projection conservatively requires a mapping for such a method. It does not infer arbitrary build-time JSON-engine substitutions or claim support for the separate `encoding/json/v2` API.

`TypeMapper` rules run before codec and structural rules. A handled rule must return a nonnil serializable schema. Its result is copied before pointer nullability, source annotations, or consumer edits can affect it; separate projections do not share the mapper's nested schema or example storage. Callback failures are returned unchanged. A callback returning `handled=false` without an error delegates to the next rule.

## Resource identity and offline dependencies

Components become root `$defs` entries. References are converted using their resolved resource scope, including URI and JSON Pointer escaping. A component with `$id` remains a separate schema resource; references to it use its resource identity. Static and dynamic anchor fragments remain anchor references. An identically spelled reference inside a different resource cannot silently bind to a root component.

Existing root `$defs`, exact JSON numbers, boolean schemas, annotations, and example values are preserved. A root definition and a component with the same name produce an error. Nil roots or components, dangling references, invalid identities, and budget overruns also return an error with no output bytes.

Provide dependencies as explicit bytes:

```go
options := compiler.StandaloneOptions{
    BaseURI: "https://example.test/schemas/request.json",
    Resources: map[string][]byte{
        "https://example.test/schemas/address.json": []byte(`{
            "$id":"address.json",
            "type":"object",
            "properties":{"city":{"type":"string"}}
        }`),
    },
}
```

Each map key is an absolute retrieval URI. Relative `$id` values resolve against their enclosing resource base. Export embeds every supplied resource under a nonconflicting root definition, including unused supplied entries. It makes resource identities absolute and rewrites retrieval aliases to those identities. Boolean dependencies receive an equivalent `allOf` wrapper so they can carry identity metadata. The final output is checked again without a separate resource map.

Only JSON Schema objects and booleans are supported as standalone dependencies. Full OpenAPI documents and raw external-example entries are rejected. No URI triggers an HTTP request, DNS lookup, or file read. OpenAPI-specific `discriminator`, `xml`, and example data remain annotations; their contents do not become schema-loading instructions or JSON Schema validation constraints.

`BaseURI` must be absolute and contain no fragment. Without an explicit base, relative identities resolve against `https://openapi.invalid/document.json`. This reserved address is never fetched. Supply your own base before combining independently exported documents in the same registry. A declared root `$id` takes precedence over the retrieval URI according to normal resource resolution.

These rules follow the resource model in [JSON Schema 2020-12 Core](https://json-schema.org/draft/2020-12/json-schema-core).

## Dialects and limits

The exporter preserves an existing `$schema`. Otherwise it uses `StandaloneOptions.Dialect`, defaulting to `https://json-schema.org/draft/2020-12/schema`. A supplied dependency without `$schema` receives the same standard 2020-12 default explicitly, so embedding it cannot accidentally change its dialect through inheritance. Dialect identifiers must be valid absolute URIs.

Preserving a custom dialect identifier does not implement that dialect's vocabulary. Indexing and structural checks cover the supported 2020-12 schema locations; unknown keywords are retained as annotation data. Consumers must provide a validator that understands their declared dialect. Do not rely on this exporter to discover references inside custom vocabulary keywords or translate legacy draft semantics.

| Option | Zero-value default | Scope |
| --- | --- | --- |
| `MaxBytes` | 8 MiB | Serialized projection plus all supplied resource bytes |
| `MaxResources` | 64 | Root, preloaded resources, and embedded `$id` resources |
| `MaxReferences` | 10000 | Reference occurrences |
| `MaxIndexBytes` | 16 MiB | Cumulative resource-index and URI-processing text |
| `MaxNormalizedBytes` | 16 MiB | Complete exported JSON, including embedded dependencies |

Negative budgets are errors. Shared JSON decoding also limits depth to 128 and nodes to 200000, and rejects duplicate keys and trailing JSON values. These limits bound processing and serialized data; they are not an exact heap-allocation measurement. The caller's original Go values already exist before export serializes them. Limit type projection separately with `ProjectionRequest.MaxTypes` and source loading with `LoadOptions.MaxPackages`.

## CLI

```sh
GOWORK=off openapi schema --dir . --type Request --projection request \
  --base-uri https://example.test/schemas/request.json \
  --output request.schema.json
```

Add `--resources resources.json` for an explicit offline manifest. Paths resolve relative to the manifest file:

```json
[
  {"uri":"https://example.test/schemas/address.json","file":"schemas/address.json"}
]
```

The manifest uses the [check command's resource format](references.md#cli), but standalone export rejects `kind: "example"`. Only explicitly listed regular files are read; schema URIs are never converted into filesystem paths.

`--dialect` selects the fallback dialect. The five resource budget flags match the SDK options in kebab case. `--max-types` defaults to 4096, `--max-packages` to 2048, and `--timeout` to one minute. Context cancellation applies to file reads and source loading and is checked before output; bounded in-memory projection and export are not interrupted at every operation. In a cold environment, run `GOWORK=off go mod download` first or allow a longer loading timeout.

Omitting `--output` writes JSON to stdout. File output is written completely to a temporary file in the destination directory and then renamed over the target. Validation, resource-read, cancellation, and rename failures leave the existing target intact and remove temporary output. The parent directory must already exist. The CLI includes its trailing newline in `--max-normalized-bytes`; SDK output has no appended newline. `schema --help` exits successfully, and unexpected positional arguments are rejected.

## Verification boundary

Tests feed the exported document alone to `jsonschema/v6` with a loader that rejects external access. They verify actual valid and invalid instances for identified components, recursive dynamic references, relative resource identities, retrieval aliases, boolean resources, and OpenAPI annotations. Component-name fuzzing checks URI and JSON Pointer escaping against that independent engine. An external Go module exercises the public API and read-only concurrent export.

These checks cover standalone export. They do not establish the complete Go codec matrix, framework analysis, custom dialect support, or final two-product acceptance.
