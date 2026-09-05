# Response effects in the public compiler SDK

Frontends translate actual framework calls or return conventions into neutral effects. The core maintains each analysis path's pending status, committed status, response headers, and body writes. It contains no Gin method or type rules.

`ResponseStatus` updates an uncommitted status. `ResponseCommit` commits headers and status immediately. `ResponseBody` commits a response and supplies its body representation. A later status cannot override a committed status; an unknown committed status stays unknown. `Abort` remains distinct from returning from a Go function. Multiple complete body writes on one path produce a diagnostic rather than response alternatives. HTTP bodyless statuses omit content instead of projecting an unused payload.

`ResponseHeader` accepts a name and string `Payload`. `DeleteHeader` removes it; `HeaderIfEmpty` writes only when the current header value is empty. Header names are case-insensitive HTTP tokens. Each branch owns an independent map, and a response records the headers present when committed. Later header calls do not change that snapshot. Known values become string `const` constraints; alternatives for one status merge with `anyOf`. Headers remain optional when presence across all paths has not been proven. Their original source facts remain in the document report.

The analyzer owns `Effect.Headers`, a map of `HeaderValue` snapshots. Frontends should emit `ResponseHeader` effects to participate in ordering instead of supplying an unordered final map. Content-Type is represented by response content keys, not a Header Object. A dynamic override or a known value inconsistent with the selected renderer produces a diagnostic; custom representation rules still require a matching frontend/codec.

`Effect.WireSchema` is an optional **response-body** representation supplied by a frontend. Without it, the core projects `Payload` using the actual Go type and codec. With it, the core detaches the schema before merging, preserving caller-owned values. This supports representations such as formatted text or raw binary; it must not be used to hide unknown business DTO structure. The frontend remains responsible for proving that its representation matches the serializer.

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
