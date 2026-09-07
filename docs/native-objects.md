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

## Tags and parent presence

`Tag.Name` is required and may be empty. Names must be unique. Summary, description, parent, and kind must be strings; kind may contain an application-defined classification. Every explicitly supplied parent must name a declared tag, and parent chains must be acyclic. The checker validates the Tag array, individual objects, and external documentation metadata while leaving extension payloads opaque.

`Tag.Parent` now uses `spec.Optional[string]` so an omitted parent differs from a reference to the empty tag name. Migrate earlier string assignments to `spec.Set(value)`, read `.Value`, and use `.Present` to inspect presence. Leave the zero value for a root tag. Null is invalid. The wire format remains a standard string:

```go
tags := []spec.Tag{
    {Name: "resources", Summary: "Resources", Kind: "nav"},
    {Name: "items", Parent: spec.Set("resources")},
}
_ = tags
```

Hierarchy checks visit each chain once within the shared reference budget. Native `summary`, `parent`, and `kind` remain in the exported document; the pinned Swagger UI's ordinary tag grouping does not constitute a native hierarchy presentation.

## Media and multipart encoding

Media and Encoding fields have separate object shapes. `encoding` is a dictionary of Encoding Objects; `prefixEncoding` is an array of Encoding Objects; `itemEncoding` is one Encoding Object. Named and positional fields cannot coexist at the same level. The same container and conflict rules apply to nested encodings. Header references inside positional and nested encodings use the normal explicit offline resource graph.

Encoding contentType must be a string; style accepts form, spaceDelimited, pipeDelimited, or deepObject. Explode and allowReserved must be booleans, including explicit false. Unknown standard fields are rejected. The media description must be a string. Extension contents are not traversed, but an encoding entry whose part name starts with `x-` is still an Encoding Object.

Positional media encoding requires `itemSchema` or structural array evidence in `schema`. The checker recognizes an array type (including a type union), `items` or `prefixItems` without a conflicting explicit type, ordinary references, and positive `allOf`/`anyOf`/`oneOf` branches. It follows local references, anchors, and supplied offline resources with a visited set and the shared budget. Unused definitions, nested properties, and reference cycles without array evidence do not establish the outer shape. Dynamic scope and arbitrary satisfiability are not solved; express an explicit array constraint or use `itemSchema` when the structure is otherwise unproven.

These are document checks, not multipart wire serialization or complete contentType grammar validation. Ignored media-specific fields and Encoding content types do not select a runtime codec. Native positional and nested encoding UI submissions have not been certified. See [Encoding by position](https://spec.openapis.org/oas/v3.2.0.html#encoding-by-position) and [Encoding Object](https://spec.openapis.org/oas/v3.2.0.html#encoding-object).

## Verification

The native object matrix compares public diagnostics with four checksummed official resources and an independent JSON Schema engine. Separate tests record normative rules that the official resources do not enforce, exercise offline external examples, and verify absent/false/true round trips across every optional boolean field. `contracttest/discriminator_test.go` covers union and inheritance context, offline anchors and back-references, cyclic traversal budgets, independent instance semantics, and actual known/unknown/omitted discriminator payloads from the native fixture.

`contracttest/xml_context_test.go` compares normative XML naming examples with the official structural schema, which accepts the missing-name counterexamples. It covers XML/JSON controls, reusable media, external schema identity, property arrays and tuples, reference wrappers, all content-bearing object roles, and traversal budgets on a cyclic graph. These checks supplement structural validation; they are not independent XML codec conformance tests.

`contracttest/tag_encoding_test.go` compares native Tag, Media, and Encoding shapes with the official structural schema, records the additional hierarchy and positional-array rules, and verifies typed parent presence, long hierarchies, reference aliases/anchors, implicit tuples, composition, and nested offline Header references.

## HTTP objects and parameter contexts

Servers require a URL-template string and string variable defaults. The checker validates template delimiters, unique variable occurrences, percent-encoded triples, forbidden query/fragment literals, nonempty string enums, and default membership. It permits relative URLs, Unicode template names, empty variable defaults and repeated enum values. It does not contact servers or certify every URL produced by substituting application-provided variable values.

Path Items support the fixed methods, including QUERY, and case-sensitive `additionalOperations` method tokens. Uppercase fixed method names cannot be repeated in `additionalOperations`; lowercase `get` is a distinct HTTP token. Operations may omit responses in native OpenAPI 3.2, but a supplied Responses Object needs at least one status/default response. The source compiler still diagnoses missing response evidence for a generated business operation. Parameter/Server arrays and callback/webhook/operation dictionaries retain their distinct containers.

Parameter and Header checking covers the location/style matrix, schema versus single-entry content, field applicability, descriptions, and explicit boolean types. A path parameter requires `required: true` even when it uses content. Querystring uses content and cannot use schema serialization fields. Required `Parameter.Name` now serializes even when empty: an empty query/cookie/querystring name is different from a missing name. Existing nonempty names keep the same wire format.

Duplicate parameter identities are checked after resolving ordinary offline references. The identity is the exact name and location. Lists on both Path Items and Operations prohibit duplicates. Operations override inherited parameters with the same identity and retain other inherited parameters. The resulting set may contain at most one querystring parameter and cannot combine it with query parameters. This applies through external Path Items, reference aliases, QUERY and custom operations. Reference-only parameter cycles report `openapi.spec.parameter.reference.cycle`. Traversal uses the shared index budget.

When a Path Item and its referenced Path Item both define the same field, the specification leaves the behavior undefined. For parameter-context checks, this checker uses the nearest explicitly defined field. It neither rewrites nor merges the serialized document. Path-template coverage, arbitrary style-versus-Schema satisfiability and runtime serializer behavior are separate checks; this context validation does not certify them.

Link parameter and requestBody values remain opaque literal data, including objects, false and null. The official pinned structural Schema rejects some non-string Link parameter values that the normative `Any` definition permits; the contract tests record that difference explicitly. `operationRef` uses the existing typed offline reference resolver. `operationId` must identify exactly one physical Operation Object across the explicitly supplied OpenAPI documents, preserving case and empty IDs. Duplicate operation IDs are rejected across those documents. Referencing one Path Item from several URLs does not duplicate its physical Operation Object; selecting its runtime URL remains an application concern.

See the normative [Parameter](https://spec.openapis.org/oas/v3.2.0.html#parameter-object), [Link](https://spec.openapis.org/oas/v3.2.0.html#link-object), and [Server](https://spec.openapis.org/oas/v3.2.0.html#server-object) contracts. A green structural schema result alone does not prove inherited parameter semantics or target identity resolution.


## Document metadata and component names

The native checker distinguishes a missing field from an explicitly empty string. `info.title`, `info.version` and `license.name` are required strings, but may be empty. The root must include at least one of `paths`, `webhooks` or `components`; an empty object satisfies that presence requirement. Unknown fixed fields, incorrect descriptive types and invalid Request Body flags produce located diagnostics. Extensions remain opaque data.

All eleven component dictionaries require names matching `^[a-zA-Z0-9.\-_]+$`. This restriction does not apply to nested Schema properties or `$defs` names. JSON Pointer escaping continues to work for arbitrary names in those locations. References retain the same validation when their complete documents are supplied as offline resources.

Contact email checks accept standalone ASCII mailboxes, quoted local parts and IPv4/IPv6 address literals. They do not test delivery or perform DNS queries. Metadata URI fields allow relative references and are checked without fetching targets. The independent backend accepts raw spaces and malformed query escapes in some URI references; the core rejects these using its own rules. The pinned official base schema also constrains the default dialect to a particular URI, whereas the native model permits syntactically valid custom dialect references; validating that dialect's semantics requires an explicit compatible Schema backend.

`license.identifier` and `license.url` are mutually exclusive by presence, even when empty. This layer checks their types and exclusivity; it does not certify SPDX expression grammar, identifier registration or legal applicability. Request Body `content` remains required and must contain at least one media entry in this implementation. OpenAPI 3.2 defines an empty content map's behavior as implementation-defined; rejection is the current explicit policy. Response descriptions remain optional.

The public tests in `contracttest/metadata_test.go` cross-check these cases against the checksum-pinned official schema and record backend differences separately from normative rules.
