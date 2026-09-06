# Independent contract validation

`contracttest` is an optional framework-neutral test package. It accepts documents, explicit resources, and actual samples without installing middleware or executing handlers or business serialization methods. JSON Schema assertions use `github.com/santhosh-tekuri/jsonschema/v6`.

## Resources and pointers

`Compile(document, pointer, Options)` accepts standalone JSON Schema or a complete OpenAPI document. The pointer must be empty or an absolute JSON Pointer selecting a real Schema location. It cannot select info, a Response Object, or business data inside examples. An empty pointer selects a standalone Schema root.

```go
validator, err := contracttest.Compile(document,
    "/components/schemas/Response",
    contracttest.Options{
        BaseURI: "https://example.test/api/openapi",
        Resources: map[string][]byte{
            "https://example.test/api/schemas/item": itemSchema,
        },
    })
if err != nil {
    return err
}
return validator.JSON(responseBody)
```

BaseURI is the retrieval URI. Resource keys are explicitly supplied absolute retrieval URIs, and values contain complete OpenAPI or JSON Schema bytes. The default base is `https://openapi.invalid/document.json`. URIs do not trigger network or file loading. Compiled results do not retain the caller's mutable resource map or byte slices.

The index preserves `$self`, nearest `$id`, retrieval aliases, anchors, and original JSON Pointer scopes. OpenAPI schemas are normalized into `$defs` for the independent engine without rounding numbers through floating point. Only actual Schema `$ref` and `$dynamicRef` keywords are rewritten; const, default, examples, extensions, and discriminator mapping names remain data.

Reference normalization reuses the core's neutral resource index. Instance assertions and dynamic reference scopes are evaluated by the independent engine. This is not a second independent reference index and does not replace complete OpenAPI document checking.

## Dynamic references and dialects

Dynamic anchor fragments retain their names rather than becoming static JSON Pointers. Tests cover recursive extensions: Strict constrains descendants through dynamic references, while the base Tree and static references retain their original field behavior. Ordinary anchors and empty fragments preserve static resolution.

The package registers fixed OpenAPI 3.1 and 3.2 dialect resources offline; see [provenance and checksums](../contracttest/dialects/PROVENANCE.md). Callers may preload custom meta-schemas. Unknown required vocabularies are rejected. `AssertFormat` and `AssertContent` explicitly select format and content assertions.

OAS descriptions, XML, and discriminator annotations do not automatically become JSON instance assertions. Normalized models are validator inputs; they do not replace the document returned by Build or change application routes.

## Budgets and limitations

| Setting | Default and scope |
| --- | --- |
| `MaxBytes` | 8 MiB for aggregate document inputs and raw JSON sample bytes. |
| `MaxResources` | 64 documents and embedded `$id` resources; retrieval and declared identities of one resource are aliases. |
| `MaxReferences` | 10,000 references; cycles do not recursively duplicate resources. |
| `MaxIndexBytes` | 16 MiB of cumulative index, URI, normalized-location, and diagnostic text processing. This is a deterministic text budget, not a heap measurement. |
| `MaxNormalizedBytes` | 16 MiB for normalized intermediate and final encoded JSON, including escaping and inserted absolute URIs. |

Documents allow at most 128 levels and 200,000 aggregate JSON nodes; duplicate keys and trailing JSON are rejected. Before reaching the numeric engine, numeric text is limited to 4,096 bytes and absolute exponents to 4,096. Excess returns `openapi.contract.budget` without exponent-sized allocation.

`Value` bounds decoded samples to 128 levels and 200,000 nodes and counts values, keys, containers, and separators. It does not invoke business serialization methods. `JSON` additionally bounds actual encoded bytes, including escaping.

Zero settings select defaults; negative settings are invalid. Resource normalization failures may report `openapi.spec.budget`; no partial Validator is returned. Complete sample duplicate-key handling, mixed inline dialects, and arbitrary custom vocabularies are outside the verified coverage.

## Stream samples

`Validator.NDJSON` splits records on LF or CRLF and rejects bare CR within a record. Empty lines are ignored, and a final complete record can end at EOF. `ParseSSE` constructs transmitted field objects using SSE CR, LF, CRLF, initial BOM, and UTF-8 decoding rules. It does not insert browser EventSource defaults or cross-event state.

`Validator.SSE` applies itemSchema to each transmitted object. JSON-formatted data remains a string; `AssertContent: true` enables inner contentSchema assertions.

Separate `Limits` bound total bytes, line bytes, and item count, including actual line endings. Configurations that overflow when boundary-check space is added are rejected. Single-byte chunk tests cover invalid UTF-8 subsequence replacement, exact line-ending budgets, overflow, and bare-CR rejection in NDJSON.
