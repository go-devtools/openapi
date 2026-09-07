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

The shared offline UI displays request/response media examples from native `dataValue` and `serializedValue`, including referenced examples and media components. JSON and `+json` data values retain their JSON types, including zero, false, null, empty collections, and strings that happen to contain JSON. Explicit wire text takes precedence without parsing, reformatting, or rounding its numbers. Paired examples also show their logical value under **Data value**. Selection and explicitly enabled submission use the displayed wire text; manual edits remain intact during unrelated response changes.

This adaptation copies component inputs only. The served document and UI source document retain the original 3.2 fields; neither is downgraded to legacy `value`. The pinned upstream assets are unchanged. Browser tests verify exact JSON/XML/plain-text request bytes, SSE response framing, references, safe text rendering, and disabled submission by default. This is not complete native UI support: parameter/header examples, form serialization, external example retrieval, and non-JSON data-only codecs still have upstream limitations. Provide `serializedValue` for exact non-JSON body examples. See the [OpenAPI Example contract](https://spec.openapis.org/oas/v3.2.0.html#example-object).

## Discriminator

`propertyName` is a required string. An empty string names the empty JSON property and is preserved by typed serialization. Mapping entries and defaultMapping identify schemas through the existing resource-aware offline graph; mappings do not change `oneOf` or `anyOf` instance validation.

The checker rejects invalid containers, non-string fields, invalid mapping containers, unresolved targets, and unknown standard fields. It also checks dispatch context after resolving offline references:

- A discriminator needs an adjacent `oneOf`, `anyOf`, or `allOf`, or an inheritance relationship in which a child uses `allOf` to reference the parent. Ordinary reference aliases and transitive inheritance are supported, including cycles within the resource budget.
- Explicit mapping and default targets must occur in the adjacent union candidates, or be descendants of the discriminator parent. List the fallback branch in the union as well. A schema's ordinary reference aliases share its resolved identity; `$id` boundaries and anchors retain the normal offline reference rules.
- When the discriminating property is not proven required, provide `defaultMapping`. A fallback that itself requires the omitted property is rejected in this case. A required property can still use a default for unrecognized present values.
- These checks do not alter instance validation: overlapping `oneOf` branches still fail, multiple `anyOf` matches still succeed, and validating an inheritance parent does not automatically validate a selected child.

Required-property proof follows explicit `required`, ordinary references, any conjunct of `allOf`, every alternative of `oneOf`/`anyOf`, and both conditional branches when `if`, `then`, and `else` are present. It is deliberately conservative: it does not solve arbitrary satisfiability constraints, infer dynamic reference scope from a static target, or infer requiredness from unrelated keywords. An unproven case receives `openapi.spec.discriminator.default.required`; provide an explicit constraint or fallback. This diagnostic is not a claim that every instance permitted by an arbitrary schema can omit that property.

Other context diagnostics are `openapi.spec.discriminator.context`, `.target`, and `.default.optional`; each identifies the offending location. The pinned official structural Schema does not enforce these rules or the normative propertyName requirement. The public checker supplements it rather than treating structural acceptance as full semantic acceptance. See [OpenAPI 3.2 Discriminator](https://spec.openapis.org/oas/v3.2.0.html#discriminator-object).

Traversal shares `CheckOptions.MaxIndexBytes` with the reference graph, so dense cyclic inheritance returns a budget diagnostic instead of expanding without a limit. Component-name mappings are resolved in their owning OpenAPI document; use explicit URI references across documents when the name could be ambiguous.

## XML

The five node kinds are element, attribute, text, cdata, and none. Namespace values must be non-relative IRIs; Unicode and fragments are allowed. A present nodeType forbids both legacy attribute and wrapped, even when either is false. The legacy wrapped field requires an adjacent array type. Unknown standard fields and wrong field types are rejected; `x-` extension values remain opaque.

The checker applies naming defaults at XML media uses (`application/xml`, `text/xml`, and structured `+xml` suffixes, with case-insensitive media types and optional parameters). It checks `content` on requests, responses, parameters, and headers, including their reusable components. A Media Type Object reference retains the media type at its use site; a reusable media component's key alone does not establish XML use.

An `element` or `attribute` needs `xml.name` unless its physical schema location supplies a component name, property name, or the property name of an enclosing array. Array items do not inherit a root component's name or an explicit wrapper name. Ordinary reference wrappers default to `none`, while their targets retain their own physical naming context, including offline `$id` and anchor targets. Unnamed inline nodes receive `openapi.spec.xml.name.required` at the schema's `/xml/name` location. Set a name or use a named component/property schema; use `nodeType: "none"` when a composition layer intentionally creates no XML node.

Static traversal follows ordinary references, named `properties`, `items`, `prefixItems`, `allOf`, `anyOf`, `oneOf`, `dependentSchemas`, and conditional `then`/`else` branches when `if` is present. It visits each reachable physical node once and shares `CheckOptions.MaxIndexBytes` with reference indexing. JSON-only use and unused schemas do not acquire XML naming requirements. `$defs` is visited only through actual references; `not`, `if`, and `propertyNames` do not create data nodes in this check.

This is a static naming check, not complete XML annotation evaluation. Dynamic reference scope, pattern/additional/unevaluated property selection, `contains`/unevaluated item selection, encoded `contentSchema`, and nested Encoding content types are not certified here. Boolean schemas and instance-dependent annotation collection do not establish a complete XML tree. XML metadata does not select a serializer or prove application XML behavior. The shared Swagger UI renders the original document with its pinned upstream capabilities; document validity does not imply complete rendering of every native keyword.

## Verification

The native object matrix compares public diagnostics with four checksummed official resources and an independent JSON Schema engine. Separate tests record normative rules that the official resources do not enforce, exercise offline external examples, and verify absent/false/true round trips across every optional boolean field. `contracttest/discriminator_test.go` covers union and inheritance context, offline anchors and back-references, cyclic traversal budgets, independent instance semantics, and actual known/unknown/omitted discriminator payloads from the native fixture.

`contracttest/xml_context_test.go` compares normative XML naming examples with the official structural schema, which accepts the missing-name counterexamples. It covers XML/JSON controls, reusable media, external schema identity, property arrays and tuples, reference wrappers, all content-bearing object roles, and traversal budgets on a cyclic graph. These checks supplement structural validation; they are not independent XML codec conformance tests.
