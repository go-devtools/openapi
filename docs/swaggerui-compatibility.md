# Offline Swagger UI compatibility

`openapi/swaggerui` embeds Swagger UI **5.32.15**. The upstream assets retain their checksums and licenses; project plugins are separate local resources. Importing the root `openapi` package does not embed this UI. Core document validity and browser rendering are different checks: a valid OpenAPI 3.2 feature may not have an interactive representation in this renderer.

## HTTP methods and tag metadata

| Feature | Browser display | Actual submission | Evidence |
| --- | --- | --- | --- |
| `query` Path Item field | Native QUERY operation, request editor and response panel | Explicit `SubmitMethods: []string{"query"}` sends `QUERY` and the exact edited JSON bytes, including Unicode and integers beyond JavaScript's exact numeric range | `internal/verify/browser/native-methods.spec.cjs` |
| `additionalOperations` | Upstream interactive operation list omits these operations. The compatibility panel lists the omitted method, path, summary and source pointer, preserving method case | Unavailable; `swaggerui.New` rejects extension names in `SubmitMethods` | Browser fixture includes distinct `SEARCH` and `search`; Go configuration checks |
| Tag `summary`, `parent`, `kind` | Upstream uses flat name/description groups. The compatibility panel includes the original values and states that grouping is flat | Not applicable | Same browser fixture includes parent and child tags, summaries and `nav` kinds |
| `in: querystring` | Read-only parameter content and an adjacent serialization limitation | Disabled per affected operation, even when its method is enabled; the underlying execution action returns `openapi.ui.request.blocked` with a diagnostic payload and sends no request | `internal/verify/browser/native-wire.spec.cjs` |
| `itemSchema` and reusable media types | Separate **Stream item schema** panels for requests and each response, following media selection; local references, boolean schemas and simultaneous whole-body Schema remain intact | Finite NDJSON uploads and NDJSON/SSE downloads preserve exact framing | Same native wire browser tests |
| Native Example values | Logical and serialized body examples, including explicit falsy values and paired representations | Exact JSON, XML and text body submissions after explicit enabling | `internal/verify/browser/native-examples.spec.cjs` |

The UI does not downgrade `openapi`, remove native fields, flatten the source tag hierarchy, or rewrite custom HTTP methods to ordinary methods. The fetched document and `window.ui.specSelectors.specJson()` retain the original content. Selecting another definition recomputes compatibility notes; notices from the previous document do not remain.

The method allowlist is empty by default. `tryItOutEnabled: false` alone is not the safety mechanism: `supportedSubmitMethods: []` prevents submission controls. Enabling QUERY does not enable GET, POST, or extension methods. QUERY verification uses a harmless local echo endpoint in Chromium; it does not prove that a production reverse proxy, server, or another browser accepts QUERY.

## Inspect browser compatibility

Native documents with measured rendering gaps show an expandable **Native OpenAPI display limitations** panel. Each warning has a stable code, severity, source rule, explanation and remedy. Method warnings also contain the exact route. JSON Pointer locations are included in the message.

The shared browser plugin provides a machine-readable report without changing state:

```js
const report = window.ui.fn.openapiUICompatibility(
  window.ui.specSelectors.specJson().toJS()
);
```

| Code | Meaning |
| --- | --- |
| `openapi.ui.additionalOperations` | This operation is missing from the interactive operation list; inspect the original document with a compatible viewer |
| `openapi.ui.tagMetadata` | The renderer does not present the named native tag fields or hierarchical grouping |
| `openapi.ui.querystring` | The renderer omits whole-query values; this integration keeps the operation read-only and blocks its request action |
| `openapi.ui.reference` | A referenced Path Item or Callback was not inspected because it could not be resolved locally |
| `openapi.ui.inspect.limit` | The scanner stopped at its depth, work or diagnostic limit; remaining content is not verified |

The inspector visits Path Item positions in paths, webhooks and callbacks, including local JSON Pointer references, root tag metadata, and whole-query parameters inherited by operations. Schema properties, Example payloads and vendor-extension contents are opaque. It never fetches references. It bounds traversal to 10,000 work steps, reference depth 64 and 200 ordinary diagnostics; hitting a bound produces a separate warning. This is a presentation report, not a replacement for `openapi.Check` or contract validation.

An empty report means that this limited scanner found no listed gap. It does **not** certify all native 3.2 features. Multipart positional encodings, XML `nodeType`, discriminator `defaultMapping`, device authorization, metadata URLs and incremental stream consumption remain separate, unverified browser combinations. See the [native support matrix](openapi32-matrix.md) for core expression and validation evidence.

## Reproduce the browser checks

Use the exact toolchain declared in the repository and install development dependencies with `npm ci`. Run `GOWORK=off make dev`, `npm run test:ui`, and `npm run test:browser`. Browser fixtures compile and start their own loopback service, use the actual embedded resources, reject every off-origin request, and fail on browser console or runtime errors. Desktop and 390-pixel mobile screenshots cover the compatibility panel; actual requests assert received method and bytes.

## Whole-query and stream boundaries

`querystring` represents the entire query value using its declared content media type and encoding. The pinned client displays its parameter editor but, in an actual GET request, omitted the value completely. This integration preserves read-only details and removes submission controls for the affected operation. The action guard also prevents a programmatic `specActions.executeRequest` call from sending that incomplete request; its returned action contains a warning in `payload`. Other operations and explicitly enabled methods remain usable. A source snapshot is cached until the selected document changes, avoiding a full document copy on every operation render.

The item panel describes each independently validated stream item. It does not convert `itemSchema` into the complete-body `schema` or an invented array contract. When both are present, the upstream whole-body Schema and the new item panel remain separate. Boolean `false` means no item is valid; it does not mean the Media Type Object or the stream is absent. Media selection retains the original logical and serialized examples.

The browser tests cover finite NDJSON upload and NDJSON/SSE response bytes. They do not establish incremental event rendering, backpressure, arbitrary streaming request support or every media codec. Whole-query native Example selection remains an upstream limitation; consult the original document for `dataValue`, `serializedValue` and encoding.
