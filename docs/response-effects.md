# Response effects in the public compiler SDK

Frontends translate actual framework calls or return conventions into neutral effects. The core maintains each analysis path's pending status, committed status, response headers, and body writes. It contains no Gin method or type rules.

`ResponseStatus` updates an uncommitted status. `ResponseCommit` commits headers and status immediately. `ResponseBody` commits a response and supplies its body representation. A later status cannot override a committed status; an unknown committed status stays unknown. `Abort` remains distinct from returning from a Go function. Multiple complete body writes on one path produce a diagnostic rather than response alternatives. HTTP bodyless statuses omit content instead of projecting an unused payload.

`ResponseHeader` accepts a name and string `Payload`. `DeleteHeader` removes it; `HeaderIfEmpty` writes only when the current header value is empty. Header names are case-insensitive HTTP tokens. Each branch owns an independent map, and a response records the headers present when committed. Later header calls do not change that wire snapshot, but remain visible in the current header storage exposed to frontends. Known values become string `const` constraints; alternatives for one status merge with `anyOf`. Headers remain optional when presence across all paths has not been proven. Their original source facts remain in the document report.

The analyzer owns `Effect.Headers`, a map of `HeaderValue` snapshots. Frontends should emit `ResponseHeader` effects to participate in ordering instead of supplying an unordered final map. Content-Type is represented by response content keys, not a Header Object. A dynamic override or a known value inconsistent with the selected renderer produces a diagnostic; custom representation rules still require a matching frontend/codec.

`Effect.WireSchema` is an optional **request or response** representation supplied by a frontend. Without it, the core projects `Payload` using the actual Go type and codec. With it, the core detaches the schema before merging, preserving caller-owned values. This supports representations such as formatted text or raw binary; it must not be used to hide unknown business DTO structure. The frontend remains responsible for proving that its representation matches the serializer.

```go
// 明确描述文本输出，业务 DTO 的 JSON 投影仍使用默认路径。
// Describe text output explicitly while keeping business DTO JSON projection on the default path.
effect := compiler.Effect{
    Kind: compiler.ResponseBody,
    Status: "200",
    MediaType: "text/plain",
    WireSchema: spec.Typed("string"),
    Source: source,
}
```

Raw HTTP bytes use a schema with `contentMediaType` and without a JSON `type` or `contentEncoding`. A JSON byte slice follows its JSON codec and may instead be base64 encoded. This distinction follows [OpenAPI 3.2 binary data semantics](https://spec.openapis.org/oas/v3.2.0.html#working-with-binary-data); arbitrary byte contents are not automatically proven to be valid JSON merely because their media type says JSON.

These effects add no new runtime imports and do not change the serialized Bundle format. Tests cover a framework-free carrier and an external module that supplies a text representation and consumes the generated header and body schemas through an independent engine. This stage does not complete method-conditioned rendering, all custom codec combinations, interim-response sequences, or the full framework analysis matrix.

## Observing response state

`CallContext.Response` contains a detached `ResponseState`: the pending/committed status, a `Committed` flag, and canonical names from the current header storage, including post-commit mutations. Each `ResponseHeaderState` separates presence from a known string value. An existing header with unknown content is still present. Frontends may use this snapshot to select effects; modifying its map cannot mutate the analyzer. Frontends continue to emit neutral effects rather than writing internal flow state.

The runtime links HEAD responses without content, preserving headers, links, summary, and description. Shared local response components are resolved and copied so GET and the original Bundle retain their complete representation. Reference summary/description overrides are retained. Unresolvable HEAD response references fail explicitly; this projection does not claim support for arbitrary external Response Object resolution.

Native `spec.Response.Summary` is available. Both summary and description are optional strings under [OpenAPI 3.2 Response Object](https://spec.openapis.org/oas/v3.2.0.html#response-object). Named component responses are distinguished from status-keyed operation responses, including names starting with `x-`.

## Ordered response items

`ResponseItem` describes one item in a sequential response and populates `MediaType.ItemSchema`. Repeated items on one execution path are permitted only when they use the same media type. Distinct shapes merge with `anyOf`, so overlapping item shapes remain valid. This does not infer the number or order of items in the final stream. Mixing complete body writes with item writes, or changing framing on one path, produces a diagnostic. Conditional linking also rejects alternatives that would erase both complete-body and item constraints. Explicit Media Type Objects may still supply both `schema` and `itemSchema` where they constrain the same sequence.

`Effect.PayloadMediaType` selects the codec used to project the Go payload; when omitted it defaults to the outer `MediaType`. For example, a JSON item in `application/x-ndjson` uses `PayloadMediaType: "application/json"` and reuses the core JSON projection, component references, and centralized type mappers. Unknown codecs and payloads remain diagnostics.

An optional `Effect.TransformSchema` callback wraps a projected response schema at compilation time. It receives a detached input, and its output is detached before merging. Returning an error or nil prevents trusted publication. The callback must be deterministic, must not execute business code, and must not retain or concurrently mutate its arguments; captured configuration must follow the frontend's existing name/version and `Options.Configuration` contract. No callback is serialized into a Bundle or used at runtime.

A frontend can wrap a JSON payload in a string schema with `contentMediaType: application/json` and `contentSchema`, then include that string as an event object's `data` property. [OpenAPI 3.2 SSE semantics](https://spec.openapis.org/oas/v3.2.0.html#special-considerations-for-server-sent-events) require protocol parsing before validation: `data` remains a string, even when its content is JSON. The external SDK test uses `contracttest.SSE` with `AssertContent: true` to check that embedded JSON retains the actual DTO constraints. Metadata absent from the wire is not made required.

HTTP 204/304 response items are discarded before projecting unused payloads. HEAD linking removes stream content while preserving response metadata. This increment verifies a neutral external frontend and independent NDJSON/SSE validation; framework-specific SSE rendering, streaming callbacks, and full protocol/UI matrices remain separate acceptance work.
