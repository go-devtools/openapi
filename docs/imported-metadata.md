# Imported metadata and source evidence

Loading an application also loads the actual types and syntax of its existing dependencies. The compiler indexes type, field, and constant metadata from that same build-selected graph. Imported DTOs retain ordinary descriptions, declared constraints, and explicitly closed enum values and labels. No second load, method execution, unused import, or additional module download is triggered by metadata lookup.

Generic instantiation creates field objects whose types have been substituted. Metadata lookup follows `go/types.Var.Origin()` to the original field declaration, while Schema projection uses the instantiated field type. A generic envelope therefore retains its Data description and presence declaration without replacing the payload with an unresolved type parameter.

## Source roots and dependencies

`Project.Packages` and `Project.Functions()` keep their existing source-root scope. Metadata indexing does not turn a dependency's functions into application operations or enable arbitrary external helper analysis. Explicitly loaded root packages still provide candidate functions to the registered frontend.

A package already imported by the application does not need to be added to `LoadOptions.Patterns` solely to preserve DTO comments. A package mentioned only in a comment is not an import; include it explicitly when loading. `TypeIn` and `Type` never discover or download missing packages from annotation text.

All matching typed constants in the loaded graph participate when a type has an explicit `@openapi enum` declaration. Without that declaration, known constants do not close a type. Values and `x-enum-descriptions` remain aligned in deterministic order. The application still owns the declared client contract; enum and length annotations do not prove that runtime input validation enforces them.

## Error scope

Malformed annotation syntax in a requested source root fails loading, as before. A malformed dependency annotation is frozen with its declaration and is reported when projection reaches the affected type, field, or closed-enum constant. Unrelated imported declarations do not prevent a valid application contract from compiling.

Selected route errors preserve the `openapi.comment.invalid` code, physical declaration source, and application call or request/response declaration in Facts. Build sets the diagnostic Route to the actual selected method and path. The same immutable Bundle can therefore report the relevant route when one handler is registered more than once.

Semantic errors such as string minimum constraints still use the shared Schema checker and retain codes such as openapi.comment.type and openapi.comment.range. Their source identifies the original field or type declaration; syntax errors identify the malformed directive token. Direct field symbols include their owning type, while anonymous nested fields refer to the real enclosing named declaration. This metadata behavior does not authorize overriding actual codec facts or replacing an unknown helper with a guessed contract.

## Source positions and loaded snapshots

Directive coordinates are captured from the exact bytes passed to the Go parser, including overlays. Go's AST comment text can normalize carriage returns; source offsets are preserved separately so CRLF and multiline block comments remain locatable. Physical filenames and coordinates are used even when a Go `//line` directive supplies a synthetic filename.

Root-source locations remain relative to the configured project directory. Dependency locations use their logical import path and filename, avoiding module-cache or checkout-directory noise. Loaded metadata is immutable: editing a source file afterward does not change an existing Project or require another read to project its schemas. Loading the changed project again participates in ordinary freshness accounting.

## Verification

The external SDK fixture exercises imported request constraints, closed and open string types, enum descriptions, instantiated generic field comments, unused malformed dependency metadata, scoped source reports, immutable concurrent projections, overlay coordinates, and physical line-directive locations. Positive and negative instances are checked with the independent contract validator. Application candidates remain restricted to selected source roots.

The same metadata feeds frontend-supplied codecs and request/response declarations. No framework-specific rules are introduced by this indexing layer. See [adapter SDK](adapter-sdk.md), [declarations](request-response-declarations.md), and [Schema annotations](schema-annotations.md).
