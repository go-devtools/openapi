# Standalone JSON Schema export

`compiler.Project.Schema` projects an actual Go type into a root schema and its component closure. `Projection.Standalone()` exports that projection as a single JSON Schema document. `StandaloneWithOptions` additionally configures resource identity, explicitly preloaded dependencies, and budgets. Neither method runs business handlers or codecs.

## Public SDK

```go
package main

import (
    "context"
    "os"

    "github.com/openapi-golang/openapi/compiler"
)

// 从真实类型导出请求契约，并显式设置可移植的资源身份。
// Export a request contract from an actual type with an explicit portable resource identity.
func main() {
    project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: "."})
    if err != nil {
        panic(err)
    }
    typ, err := project.Type("Request")
    if err != nil {
        panic(err)
    }
    projection, err := project.Schema(compiler.ProjectionRequest{
        Type: typ, Direction: compiler.Input,
    })
    if err != nil {
        panic(err)
    }
    raw, err := projection.StandaloneWithOptions(compiler.StandaloneOptions{
        BaseURI: "https://example.test/schemas/request.json",
        MaxNormalizedBytes: 1 << 20,
    })
    if err != nil {
        panic(err)
    }
    if _, err = os.Stdout.Write(raw); err != nil {
        panic(err)
    }
}
```

The current package must declare `Request`, or the caller must select another loaded type. Use `compiler.Output` for a response projection. The two directions follow their respective wire contracts; exporting a schema does not prove that a server enforces every declared constraint.

The returned bytes do not alias the projection. Concurrent exports may read the same projection and resource map, provided the caller does not modify either input during those calls. Export does not retain or modify caller-owned resource bytes.

## Resource identity and offline dependencies

Components become root `$defs` entries. References are converted using their resolved resource scope, including URI and JSON Pointer escaping. A component with `$id` remains a separate schema resource; references to it use its resource identity. Static and dynamic anchor fragments remain anchor references. An identically spelled reference inside a different resource cannot silently bind to a root component.

Existing root `$defs`, exact JSON numbers, boolean schemas, annotations, and example values are preserved. A root definition and a component with the same name produce an error. Nil roots or components, dangling references, invalid identities, and budget overruns also return an error with no output bytes.

Provide dependencies as explicit bytes:

```go
options := compiler.StandaloneOptions{
    BaseURI: "https://example.test/schemas/request.json",
    Resources: map[string][]byte{
        "https://example.test/schemas/address.json": []byte(`{
            "$id":"address.json",
            "type":"object",
            "properties":{"city":{"type":"string"}}
        }`),
    },
}
```

Each map key is an absolute retrieval URI. Relative `$id` values resolve against their enclosing resource base. Export embeds every supplied resource under a nonconflicting root definition, including unused supplied entries. It makes resource identities absolute and rewrites retrieval aliases to those identities. Boolean dependencies receive an equivalent `allOf` wrapper so they can carry identity metadata. The final output is checked again without a separate resource map.

Only JSON Schema objects and booleans are supported as standalone dependencies. Full OpenAPI documents and raw external-example entries are rejected. No URI triggers an HTTP request, DNS lookup, or file read. OpenAPI-specific `discriminator`, `xml`, and example data remain annotations; their contents do not become schema-loading instructions or JSON Schema validation constraints.

`BaseURI` must be absolute and contain no fragment. Without an explicit base, relative identities resolve against `https://openapi.invalid/document.json`. This reserved address is never fetched. Supply your own base before combining independently exported documents in the same registry. A declared root `$id` takes precedence over the retrieval URI according to normal resource resolution.

These rules follow the resource model in [JSON Schema 2020-12 Core](https://json-schema.org/draft/2020-12/json-schema-core).

## Dialects and limits

The exporter preserves an existing `$schema`. Otherwise it uses `StandaloneOptions.Dialect`, defaulting to `https://json-schema.org/draft/2020-12/schema`. A supplied dependency without `$schema` receives the same standard 2020-12 default explicitly, so embedding it cannot accidentally change its dialect through inheritance. Dialect identifiers must be valid absolute URIs.

Preserving a custom dialect identifier does not implement that dialect's vocabulary. Indexing and structural checks cover the supported 2020-12 schema locations; unknown keywords are retained as annotation data. Consumers must provide a validator that understands their declared dialect. Do not rely on this exporter to discover references inside custom vocabulary keywords or translate legacy draft semantics.

| Option | Zero-value default | Scope |
| --- | --- | --- |
| `MaxBytes` | 8 MiB | Serialized projection plus all supplied resource bytes |
| `MaxResources` | 64 | Root, preloaded resources, and embedded `$id` resources |
| `MaxReferences` | 10000 | Reference occurrences |
| `MaxIndexBytes` | 16 MiB | Cumulative resource-index and URI-processing text |
| `MaxNormalizedBytes` | 16 MiB | Complete exported JSON, including embedded dependencies |

Negative budgets are errors. Shared JSON decoding also limits depth to 128 and nodes to 200000, and rejects duplicate keys and trailing JSON values. These limits bound processing and serialized data; they are not an exact heap-allocation measurement. The caller's original Go values already exist before export serializes them. Limit type projection separately with `ProjectionRequest.MaxTypes` and source loading with `LoadOptions.MaxPackages`.

## CLI

```sh
GOWORK=off openapi schema --dir . --type Request --projection request \
  --base-uri https://example.test/schemas/request.json \
  --output request.schema.json
```

Add `--resources resources.json` for an explicit offline manifest. Paths resolve relative to the manifest file:

```json
[
  {"uri":"https://example.test/schemas/address.json","file":"schemas/address.json"}
]
```

The manifest uses the [check command's resource format](references.md#命令行), but standalone export rejects `kind: "example"`. Only explicitly listed regular files are read; schema URIs are never converted into filesystem paths.

`--dialect` selects the fallback dialect. The five resource budget flags match the SDK options in kebab case. `--max-types` defaults to 4096, `--max-packages` to 2048, and `--timeout` to one minute. Context cancellation applies to file reads and source loading and is checked before output; bounded in-memory projection and export are not interrupted at every operation. In a cold environment, run `GOWORK=off go mod download` first or allow a longer loading timeout.

Omitting `--output` writes JSON to stdout. File output is written completely to a temporary file in the destination directory and then renamed over the target. Validation, resource-read, cancellation, and rename failures leave the existing target intact and remove temporary output. The parent directory must already exist. The CLI includes its trailing newline in `--max-normalized-bytes`; SDK output has no appended newline. `schema --help` exits successfully, and unexpected positional arguments are rejected.

## Verification boundary

Tests feed the exported document alone to `jsonschema/v6` with a loader that rejects external access. They verify actual valid and invalid instances for identified components, recursive dynamic references, relative resource identities, retrieval aliases, boolean resources, and OpenAPI annotations. Component-name fuzzing checks URI and JSON Pointer escaping against that independent engine. An external Go module exercises the public API and read-only concurrent export.

These checks cover standalone export. They do not establish the complete Go codec matrix, framework analysis, custom dialect support, or final two-product acceptance.
