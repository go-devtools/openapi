# Offline references and resource budgets

The core supports OpenAPI 3.2 document bases through `$self`, Schema resources through `$id`, anchors, and initial target resolution for `$ref` and `$dynamicRef`. The rules follow [OpenAPI base URI and reference resolution](https://spec.openapis.org/oas/v3.2.0.html#appendix-f-examples-of-base-uri-determination-and-reference-resolution) and [JSON Schema 2020-12 Core](https://json-schema.org/draft/2020-12/json-schema-core).

## Go API

`openapi.Check(data)` uses defaults. `CheckWithOptions(data, options)` accepts explicit offline content and returns the same `Report` type. The checker does not invoke file, HTTP, DNS, or redirect loaders.

```go
package main

import (
    "fmt"

    "github.com/openapi-golang/openapi"
)

// Check only documents and schemas explicitly provided in memory.
func main() {
    document := []byte(`{
      "openapi":"3.2.0",
      "$self":"https://example.test/api/openapi.json",
      "info":{"title":"Offline example","version":"1"},
      "components":{"schemas":{"Item":{"$ref":"schemas/item"}}}
    }`)
    options := openapi.CheckOptions{
        Resources: map[string][]byte{
            "https://example.test/api/schemas/item": []byte(`{
              "type":"object",
              "properties":{"name":{"type":"string"}}
            }`),
        },
    }
    report := openapi.CheckWithOptions(document, options)
    if report.HasErrors() {
        panic(report)
    }
    fmt.Println("valid")
}
```

| Field | Behavior |
| --- | --- |
| `BaseURI` | Absolute fragment-free retrieval URI for the main document. Omission selects an internal base for this check only. |
| `Resources` | Absolute retrieval URIs mapped to complete JSON bytes. A root containing openapi is checked as OpenAPI; other objects and booleans are checked as JSON Schema. |
| `ExampleResources` | Raw bytes for externalValue. Their presence proves that content was provided; embedded `$ref` text is not a loading instruction. |
| `MaxBytes` | Aggregate main document and preloaded bytes; default 8 MiB. |
| `MaxResources` | Main, preloaded, and embedded `$id` resources; default 64. A retrieval URI and the same resource's declared URI count once. |
| `MaxReferences` | Reference occurrences; default 10,000. Repeated references are counted and reported at each location. |
| `MaxIndexBytes` | Cumulative text processing for index paths, resource URIs, resolution, and diagnostics; default 16 MiB. This is not an exact heap allocation measurement. |

Zero budgets select defaults; negative budgets produce `openapi.spec.options`. All JSON shares a 200,000-node budget, with at most 128 levels per document. Duplicate keys and trailing JSON values are rejected. Exceeding a resource, reference, or input limit produces `openapi.spec.budget`; truncated results are not complete.

Input maps and bytes are read-only during a call and must not be mutated concurrently. They are not retained afterward. Independent checks do not share a mutable resource graph.

## Resolution rules

Relative `$self` resolves against the retrieval URI. Relative `$id` resolves against the nearest base and creates a resource boundary inherited by descendants. `$id` cannot have a nonempty fragment. Anchors belong to their resource and cannot borrow a same-named anchor elsewhere.

JSON Pointer resolution decodes the URI fragment, then strictly processes `~0`, `~1`, and array indices. Targets must have the expected specification object kind. An existing info object is not a valid Schema target. A pointer crossing the target Schema's `$id` produces `openapi.spec.ref.scope`; reference the nearest resource URI instead.

Discriminator component-name mappings identify the named component directly, using its `$id` when present. They are not misclassified as cross-resource JSON Pointers.

Cycles are checked as finite edges without expanding recursive schemas. Checking `$dynamicRef` establishes only that its initial target exists and has the right kind; instance-time dynamic scope belongs to a JSON Schema validator. This API does not validate instances or prove that handlers enforce declarations.

All explicitly preloaded specification documents undergo structure and reference checking, including unreferenced entries. Conflicting retrieval URIs, declared identities, and anchors produce errors. External examples are checked for resource availability; media compatibility and instance validity require separate validation.

Resource roots support complete OpenAPI objects and JSON Schema objects or booleans. Keep a Response Object inside a complete OpenAPI resource and reference it with a JSON Pointer; a fragment is not guessed to be a complete document.

## Build and pruning

`openapi.Config.Validation` passes the same options to pruning and final validation. The closure follows `$id`, anchors, ordinary references, initial dynamic-reference targets, and discriminator mappings. Referencing a node inside a component preserves the whole component. `$ref` text in extensions, defaults, or examples does not preserve unrelated components.

An external resource referencing a local component brings that component into the closure. Missing resources reachable only from unselected local models do not contaminate the final document. Global identity conflicts and budget failures still block construction.

These options configure checking. They do not inline resources into `Document.JSON()` or publish URLs. Gin Mount does not currently map preloaded resources to UI endpoints, so offline resolution does not establish browser availability of external references.

## CLI

```sh
openapi check --spec openapi.json \
  --base-uri https://example.test/api/openapi.json \
  --resources resources.json \
  --max-bytes 8388608 --max-resources 64 --max-references 10000 \
  --max-index-bytes 16777216
```

Example manifest:

```json
[
  {"uri":"https://example.test/api/schemas/item","file":"schemas/item.json"},
  {"uri":"https://example.test/api/examples/message.txt","file":"examples/message.txt","kind":"example"}
]
```

An omitted kind or document selects Resources; example selects ExampleResources. Relative file paths resolve from the manifest directory. Only explicitly named regular files are read. Document URIs are never converted into file paths. The manifest is limited to 1 MiB; main and preloaded files share the byte budget.

Missing files, duplicate URIs, unknown fields or kinds, and trailing JSON are rejected. Cancellation is checked before and between reads. Successful checking and `check --help` exit 0. Document or resource errors exit nonzero with JSON diagnostics.
