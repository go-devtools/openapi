# Native OpenAPI 3.2 acceptance matrix

This matrix tracks the complete Goal scope. Model availability, serialization, semantic validation, independent validation, and UI rendering are separate claims. An existing broad fixture does not prove every positive/negative combination. Pending cells remain required acceptance work.

| Area | Public expression/model | Current evidence | Remaining acceptance |
| --- | --- | --- | --- |
| Root, `$self`, dialect, references | `spec.OpenAPI`, `spec.Ref`, Schema references | `spec/model_test.go`, offline reference tests and `docs/references.md` | Complete base-URI and external-reference matrix; UI behavior |
| HTTP methods | `spec.PathItem.Query`, `AdditionalOperations` | Native fixture and duplicate-method validation tests | All method-conflict and UI combinations |
| Parameters | `spec.Parameter` and public compiler effects | Querystring fixture, parameter codec tests, external SDK | Complete style/content/location positive and negative matrix |
| Media types | `spec.MediaType`, shared component media types | Native roundtrip fixture and media references | Item/schema/ref independent and UI matrix |
| Multipart | `spec.Encoding`, prefix/item encoding | Native fixture; actual raw-field and file tests through public SDK | All combination constraints and UI projections |
| Responses and HEAD | `spec.Response.Summary`, optional description, headers/content/links | `TestHEADResponseProjection`, `TestNamedResponseComponents`, real Gin HTTP matrix | External Response Object resolution for HEAD; complete independent/UI matrix |
| Tags | `spec.Tag` | Native fixture, cyclic-parent validation, example groups | Full hierarchy/kind errors and native hierarchy UI |
| Examples | `spec.Example`, Schema examples | Native fixture and existing offline UI tests | Every value/dataValue/serializedValue/externalValue combination |
| Discriminator | `spec.Discriminator` | Native model roundtrip fixture | Complete semantic and independent polymorphism checks |
| XML | `spec.XML` | Native model roundtrip fixture | Every nodeType constraint, wire codec and UI combination |
| Security | `spec.SecurityScheme`, OAuth models | Native fixture and Bearer-only business example | Complete device authorization/deprecation/metadata validation and UI |
| Callbacks, webhooks, links, servers | Public typed models | Full native fixture, callback/link reference-context tests | Full positive/negative and external resource matrix |
| JSON Schema | `spec.Schema` and public Schema compiler | Schema tests, independent contract engine, external SDK | Complete Go projection/tag/codec/numeric/nullability matrix |
| Streams | `spec.MediaType.ItemSchema` and Schema content fields | Native model fixture | Full real NDJSON/SSE samples, compiler derivation and UI |

The current HTTP response increment corrected a 3.1-era assumption during review: Response.description is optional in 3.2. The validator checks the types of present summary/description fields. Component response names are not HTTP status codes, and an `x-` component name must still be validated as a response.

Go 1.27.1 development, race, fixed-remote and UI evidence is recorded in [verification.md](verification.md). The full Goal remains active until all required cells have authoritative evidence.
