# Request-field effects

A frontend can emit `compiler.RequestField` to describe one named field in a request body without inventing a DTO or struct tags. `Name` and `MediaType` must be explicit. Set `Payload` and a suitable `WireCodec` to reuse shared Go type and annotation projection, or supply a proven `WireSchema`. `Encoding` contains the standard OpenAPI Encoding Object for that field. The compiler copies supplied schemas and encodings before merging them.

`Required` on a RequestField requires the property when the containing body is present. It does not make the entire request body required. A separate RequestBody effect carries body-level required. Within one execution path, request-field properties and whole-body projections are combined together; whole-body and partial-field views use `allOf` rather than weakening either view with alternatives. Duplicate identical fields are deduplicated; incompatible schemas or encodings for one field produce a diagnostic.

Different execution paths contribute alternative body schemas. Body-level required is true only when every contributing execution path requires a body, including paths with no body reads. Finite method/media conditions retain their independent path groups for runtime linking. This composition rule does not infer that every binding error rejects empty bodies or that a field getter enforces presence; those require additional control-flow evidence.

Request-field encodings are merged by property name across compatible conditions. Distinct names can coexist; conflicting encodings for the same name fail. Existing prefixEncoding/itemEncoding and other incompatible metadata remain subject to the ordinary condition checks. The [OpenAPI 3.2 Encoding Object](https://spec.openapis.org/oas/v3.2.0.html#encoding-object) defines the wire serialization; schema validation of logical values alone does not verify multipart or URL encoding.

`ParameterRead` also accepts explicit WireSchema and retains Style/Explode. A wire array of text values must not inherit JSON slice nullability or Base64 conventions accidentally. Conflicting repeated parameter projections are diagnosed instead of silently keeping the first observation.

All these are transport-neutral compiler effects. Framework method recognition, form-versus-query precedence, file getter behavior, and method-dependent body parsing belong to the adapter. No new runtime dependency on the compiler is introduced. The Bundle continues to contain ordinary OpenAPI schemas, encodings, and finite conditions, not callback objects or go/types values.

Development and independent-consumer tests compile real source and use an independent validator for conjoined field constraints, whole-body intersections, and body presence across both path visitation orders. Runtime tests cover compatible field encodings and conflicting encodings. This guide does not claim complete body-required inference, every codec/tag combination, or the full product acceptance matrix.
