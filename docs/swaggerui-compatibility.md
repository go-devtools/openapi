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
| Security deprecation, OAuth metadata and device flows | Native endpoints, metadata reference, deprecation and scopes in the authorization dialog | Device grant and automatic discovery are unavailable and diagnosed; the device panel has no grant action. Explicit Bearer submission remains supported | `internal/verify/browser/native-security.spec.cjs` |
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
| `openapi.ui.webhooks` | The named webhook is absent from the operation list; inspect its original Path Item with a webhook-capable viewer |
| `openapi.ui.additionalOperations` | This operation is missing from the interactive operation list; inspect the original document with a compatible viewer |
| `openapi.ui.tagMetadata` | The renderer does not present the named native tag fields or hierarchical grouping |
| `openapi.ui.querystring` | The renderer omits whole-query values; this integration keeps the operation read-only and blocks its request action |
| `openapi.ui.deviceAuthorization` | Device flow details are available, but this viewer cannot perform the grant |
| `openapi.ui.oauth2Metadata` | Metadata is shown as a reference without retrieval or automatic configuration |
| `openapi.ui.reference` | A referenced Path Item, Callback or Security Scheme was not inspected because it could not be resolved locally |
| `openapi.ui.inspect.limit` | The scanner stopped at its depth, work or diagnostic limit; remaining content is not verified |

The inspector visits Path Item positions in paths, webhooks and callbacks, including local JSON Pointer references, root tag metadata, whole-query parameters inherited by operations, and component security schemes. Schema properties, Example payloads and vendor-extension contents are opaque. It never fetches references. It bounds traversal to 10,000 work steps, reference depth 64 and 200 ordinary diagnostics; hitting a bound produces a separate warning. This is a presentation report, not a replacement for `openapi.Check` or contract validation.

An empty report means that this limited scanner found no listed gap. It does **not** certify all native 3.2 features. Request-body encoding is checked separately for the currently selected media type, as described below. Incremental response display is unavailable in this viewer, as measured below. See the [native support matrix](openapi32-matrix.md) for core expression and validation evidence.

## Reproduce the browser checks

Use the exact toolchain declared in the repository and install development dependencies with `npm ci`. Run `GOWORK=off make dev`, `npm run test:ui`, and `npm run test:browser`. Browser fixtures compile and start their own loopback service, use the actual embedded resources, reject every off-origin request, and fail on browser console or runtime errors. Desktop and 390-pixel mobile screenshots cover the compatibility panel; actual requests assert received method and bytes.

## Whole-query and stream boundaries

`querystring` represents the entire query value using its declared content media type and encoding. The pinned client displays its parameter editor but, in an actual GET request, omitted the value completely. This integration preserves read-only details and removes submission controls for the affected operation. The action guard also prevents a programmatic `specActions.executeRequest` call from sending that incomplete request; its returned action contains a warning in `payload`. Other operations and explicitly enabled methods remain usable. A source snapshot is cached until the selected document changes, avoiding a full document copy on every operation render.

The item panel describes each independently validated stream item. It does not convert `itemSchema` into the complete-body `schema` or an invented array contract. When both are present, the upstream whole-body Schema and the new item panel remain separate. Boolean `false` means no item is valid; it does not mean the Media Type Object or the stream is absent. Media selection retains the original logical and serialized examples.

The browser tests cover finite NDJSON upload and NDJSON/SSE response bytes. A separate gated-response test observes nonzero network chunk bytes while the response remains open, verifies that no live response body is displayed, then explicitly releases the second item and checks the complete displayed body. The response item panel states that bodies appear only after completion. This viewer does not provide incremental event rendering; use a streaming client for long-lived responses. These checks do not establish backpressure, arbitrary streaming request support or every media codec. Whole-query native Example selection remains an upstream limitation; consult the original document for `dataValue`, `serializedValue` and encoding.

## Native security metadata

The authorization dialog reads `deprecated`, `oauth2MetadataUrl` and `flows.deviceAuthorization` from the original OpenAPI 3.2 document, including bounded local Security Scheme references. It displays the device authorization, token and optional refresh endpoints and scope descriptions. `deprecated: true` adds a label; false and omission do not. Deprecation does not prevent an otherwise supported authorization method.

The pinned OAuth form does not implement the device grant. Its device panel therefore contains read-only details, a limitation notice and a Close button, without a misleading Authorize action. OAuth metadata URLs are plain text references: the viewer does not retrieve them, initiate discovery or change its same-origin CSP. Existing redirect OAuth forms remain available with metadata annotations. The browser tests inspect their controls; they do not perform a production redirect or token exchange.

A mixed native fixture verifies device display, false/true/absent deprecation, document switching, literal scope descriptions, desktop/mobile wrapping and a real local Bearer-authenticated JSON request. The Gin basic example continues to offer Bearer only. These checks do not establish device polling, refresh, external authorization-server interoperability or credential persistence.

## Parameters, response headers, forms and media changes

Native `dataValue` examples populate ordinary parameter controls as logical data, so the request serializer encodes them once. A query containing a space and a numeric-zero header are verified against actual browser request URLs and headers. The parameter adapter does not treat `serializedValue` as an already-decoded input. Whole-query parameters remain subject to the restriction above.

Native form `dataValue` objects fill editable fields and expose a **Form example** selector. Explicit false and zero values are retained. Choosing another example replaces the form values; typing afterwards is preserved. A paired `serializedValue` is shown as reference wire text, while submission uses the editable logical fields and the declared form encoding. Equivalent percent-encoding is not required to match the reference string byte for byte. The browser suite verifies URL-encoded form values and transitions between form and JSON bodies; arbitrary file and nested multipart forms are not established by these checks.

Response Header examples have a separate expandable display for data, serialized text and external references. External header examples are reference text and are not fetched. The original header description/type table remains available. Native body defaults participate in reset and media-change decisions, so a default from one media type is not mistaken for a manual edit and copied into another format.

## Selected request encoding limits

`internal/verify/browser/native-projection.spec.cjs` measures these boundaries using the real embedded client and a local byte-echo endpoint:

| Selected media | Behavior |
| --- | --- |
| Multipart other than `multipart/form-data`, or positional `prefixEncoding` / `itemEncoding` | Shows the declared encoding metadata and disables Execute. The pinned client otherwise sends a JSON array with a multipart Content-Type and no boundary |
| XML with native `nodeType` annotations and no selected `serializedValue` example | Shows the native node paths and a sampler limitation; Execute is disabled because the generated XML ignores those annotations |
| XML with an explicit `serializedValue` example | Uses the declared wire text; actual attributes and CDATA bytes are verified |
| Supported alternative such as JSON or URL-encoded form | Remains available when explicitly enabled; switching back to unsupported media restores its restriction |

The request action is guarded as well as its Execute button. A blocked action returns `openapi.ui.request.blocked` with `openapi.ui.multipart` or `openapi.ui.xmlNodeType` in its payload and sends no request. The source specification is never rewritten. This does not add a general XML or positional multipart codec.

For a selected path, method, media type and optional example name, the browser exposes a read-only decision separately from the document-level compatibility report:

```js
const limitation = window.ui.fn.openapiBodyLimitation(
  window.ui.specSelectors.specJson().toJS(),
  "/items", "post", "application/xml", "exampleName"
);
```

A non-null result contains a stable code, message, fix and relevant metadata. The lookup follows bounded local references and does not retrieve external resources. These guards cover native request bodies under Paths; an empty decision is not a general wire-format validator.

## Schema metadata

Compact Schema `examples` preserve the upstream Discriminator, XML, external documentation and legacy `example` displays. Discriminator mappings display the referenced component's declared `title` when available, while the source references and schema identities remain unchanged. Unknown and external mappings remain literal reference text; the viewer does not retrieve mapping targets.

Native `defaultMapping` is shown with its fallback meaning for a missing or unmatched discriminator value. This is descriptive metadata and does not select a different validation result. XML `nodeType` appears alongside the existing XML name, namespace and prefix annotations; this display does not add XML serialization support.

`internal/verify/browser/native-schema-metadata.spec.cjs` verifies the rendered 3.2 metadata, 3.1 compatibility without native fields, compact false-valued examples, model collapse/reopen, desktop/mobile layout and an unchanged source document. External documentation retains its upstream link and description without automatic retrieval. Unit checks additionally cover empty discriminator property names, unknown/external mappings and all five XML node modes.

## Callbacks, webhooks, links and servers

`internal/verify/browser/native-relationships.spec.cjs` loads reusable Callback and Link objects without rewriting the served document. The Callback tab displays the runtime URL expression, method, referenced request schema and response. Its nested operation has no Execute action; the enclosing subscription request remains separate. This verifies documentation display, not callback delivery or a receiving service.

Response links show the target `operationId`, description and parameter expressions. They do not automatically execute the linked operation or evaluate a response expression into another request. Root and operation-level server choices are both visible; operation-level options are labeled as overriding global options. The tests verify those choices, not an external deployment or every server-template substitution.

The pinned renderer omits root webhooks from its operation list. A native compatibility warning identifies each webhook and its original source pointer, including referenced and webhook-only documents. The integration does not invent a Paths operation to represent a webhook. Source definitions remain available through the original OpenAPI document. Long link expressions and callback controls wrap within the 390-pixel mobile viewport.

The gated NDJSON/SSE response check is in `internal/verify/browser/native-incremental.spec.cjs`. Its service flushes the first item and waits for an explicit local release; the browser test observes actual network data and checks that the request is still unfinished. This distinguishes complete-response display from an incremental stream reader without relying on a fixed server sleep.
