# Native objects and explicit values

The `spec` package models OpenAPI 3.2 directly. `openapi.Check` validates supplied JSON offline before consumers rely on the typed model. Typed construction and JSON decoding do not replace document validation.

## Boolean API migration

All optional standard boolean fields use `spec.Optional[bool]`. This includes Operation deprecation, Parameter flags, RequestBody required, Encoding allowReserved, Header flags, Schema deprecated/readOnly/writeOnly/uniqueItems, and legacy XML attribute/wrapped. Existing explode and SecurityScheme deprecated fields use the same representation.

```go
operation := spec.Operation{Deprecated: spec.Set(false)}
body := spec.RequestBody{Required: spec.Set(true)}
xml := spec.XML{Wrapped: spec.Set(false)}
_ = operation
_ = body
_ = xml
```

When upgrading from the earlier pre-1.0 API, replace boolean field assignments with `spec.Set(value)` and read `.Value`. Inspect `.Present` when absence differs from explicit false. Leave the field zero-valued to omit it. This is a Go source API change; the JSON field names and boolean representations remain standard.

`Optional` decoding preserves explicit null for nullable types and arbitrary logical values. It rejects null for nonnullable scalar types, preserves JSON number precision for `any`, and leaves the previous value unchanged when decoding fails.

## Example

`dataValue` holds the logical value; `serializedValue` holds the wire representation as a string. They may appear together. `externalValue` can accompany `dataValue`, but cannot accompany `serializedValue`. The legacy `value` field is mutually exclusive with all three. Logical values and extensions are data, including false, zero, empty collections, and null.

```go
example := spec.Example{
    DataValue: spec.Set[any](nil),
    SerializedValue: spec.Set("<item/>"),
}
_ = example
```

Summary, description, serializedValue, and externalValue must be strings when present. External example bytes must be supplied explicitly through `CheckOptions.ExampleResources`; checking never downloads them. See [offline references](references.md).

## Discriminator

`propertyName` is a required string. An empty string names the empty JSON property and is preserved by typed serialization. Mapping entries and defaultMapping identify schemas through the existing resource-aware offline graph; mappings do not change `oneOf` or `anyOf` instance validation.

The checker rejects invalid containers, non-string fields, invalid mapping containers, unresolved targets, and unknown standard fields. The pinned official Schema omits the normative propertyName requirement, so the public checker supplements it with a located diagnostic. Composite inheritance and proving whether a property can be omitted require additional semantic context; the current structure checks do not certify those conditions.

## XML

The five node kinds are element, attribute, text, cdata, and none. Namespace values must be non-relative IRIs; Unicode and fragments are allowed. A present nodeType forbids both legacy attribute and wrapped, even when either is false. The legacy wrapped field requires an adjacent array type. Unknown standard fields and wrong field types are rejected; `x-` extension values remain opaque.

Name inference across XML use sites and complete discriminator inheritance checks are not yet certified by this checker. XML metadata does not select a serializer or prove application XML behavior. The shared Swagger UI renders the original document with its pinned upstream capabilities; document validity does not imply complete rendering of every native keyword.

## Verification

The native object matrix compares public diagnostics with four checksummed official resources and an independent JSON Schema engine. Separate tests record normative rules that the official resources do not enforce, exercise offline external examples, and verify absent/false/true round trips across every optional boolean field.
