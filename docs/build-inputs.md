# Build inputs and generation freshness

The compiler records the target selected by the Go command used for `packages.Load`: Go version, GOOS, GOARCH, CGO_ENABLED, GOEXPERIMENT, architecture feature settings, and effective tags. It does not copy the generator host's GOOS/GOARCH into the target profile. Default loading uses `-mod=readonly`; explicit vendor mode is allowed. Writable module modes, executable `-toolexec` wrappers, and external packages drivers fail before package loading. The builtin packages driver is selected explicitly. Supply overlays through `LoadOptions.Overlay`, using absolute keys; command-line `-overlay` files are rejected so source snapshots remain attributable.

Input identities combine package import paths with paths inside each package. Digests cover Go source and comments, inactive files reported by Go, non-Go package sources, embedded resources, and the effective module records for loaded packages. Module records retain selected and replacement versions plus a digest of normalized module declarations. Local replacement and workspace `use` directories are normalized to logical module identities. Local workspace dependency source is included directly; a declared version alone is insufficient.

Parsed source is fingerprinted from the bytes actually supplied to the parser, including overlays. Files present only in an overlay do not need to exist on disk, and loading never writes those bytes to business source. Supplemental source and module inputs share `LoadOptions.MaxSourceBytes`, which defaults to 128 MiB. Exhaustion is a diagnostic, not successful partial analysis. The owned `zz_openapi.gen.go` is excluded; unrelated output files are not package inputs. Loaded source and the resulting projected contracts both contribute to the final fingerprint.

Build parameter values and custom configuration may contain deployment details. Bundle profiles contain parameter/configuration digests rather than the original values. Target selectors and codec names remain inspectable. A fingerprint records inputs; it does not prove that runtime code matches source that is no longer available. CI must reload source and use `Result.Check` or the adapter's `check` command.

## Custom compilation settings

Frontend names and codec names must identify their rules and configuration stably. Do not derive them from function pointers, addresses, timestamps, or random values. `Options.Configuration` supplies additional named JSON inputs for captured settings and centralized mappings. Compile calls containing TypeMapper functions must provide a nonempty configuration declaration, since arbitrary closures cannot be reconstructed from runtime addresses.

```go
// 集中映射配置参与新鲜度，原始值不会进入公开 Bundle。
// Include centralized mapping settings in freshness without exposing raw values in the public Bundle.
options := compiler.Options{
    Load: compiler.LoadOptions{Dir: "."},
    Frontends: []compiler.Frontend{frontend},
    Mappers: []compiler.TypeMapper{mapper},
    Configuration: map[string]json.RawMessage{
        "parameter-decoding": json.RawMessage(`{"version":1,"minLength":2}`),
    },
}
```

The caller must update these declarations when externally supplied mapping or codec configuration changes. The compiler does not execute a custom decoder to discover settings. Schemas remain derived or declared through the public projection APIs.

## Runtime build checks

`openapi.CheckRuntimeBuild(profile)` compares a Bundle profile with build information embedded in the current executable. It reads no source files, executes no analyzer, and does not use process environment variables as build evidence. `openapi.Build` enables the same checks with `Config.VerifyRuntimeBuild: true`; its default permits cross-target offline exports. The Gin adapter enables the check automatically when building or mounting documents for a running engine.

Known GOOS, GOARCH, CGO, architecture selectors, explicit tags, and experiment selections must agree. Tag order and duplicates are ignored. Different Go language versions fail; differing toolchain versions within the same Go language version produce a warning, rather than an equality-based compatibility gate. Experiment selections are compared as recorded strings; equivalent spellings are not inferred. Only selectors relevant to the chosen architecture are recorded.

`BuildProfile.Settings` distinguishes an absent key (unknown) from a present empty value (known default). The compiler explicitly records empty tags and experiment settings. The verified Go 1.27 gc build-info writer omits default tags, default experiments, and disabled FIPS; the runtime recognizes those defaults only when that writer and its target metadata are identifiable. Other missing settings produce `openapi.build.unknown` warnings. Legacy Bundles without build selectors remain readable and produce `openapi.build.unrecorded` warnings when runtime validation is requested.

Known differences produce `openapi.build.mismatch` errors before document configuration or Gin documentation route registration. Warnings remain in `Document.Report()`. None of these checks prove source equality, compare complete module graphs, verify arbitrary compiler flags, or replace source-based freshness checks in CI. Runtime dependency trees still exclude the compiler, framework rules, UI assets, and independent contract validator.

## Verified boundaries and remaining work

Tests run actual Go loading for separate target selectors, overlay changes and new overlay files, same-named files in different packages, inactive source, selected local replacement versions, embedded assets, local workspace mutations, relocation, read-only module files, input exhaustion, custom mapping identity, and linker-value confidentiality. A harmless marker driver demonstrated the old execution path before the driver guard was implemented. Public external SDK consumers repeat the input tests without importing core internal packages.

This stage does not claim every runtime/profile or compiler flag/toolchain combination, arbitrary custom C toolchains and external header graphs, or the full product goal. The complete schema/helper/codec matrices, remaining build-condition matrices, CI, and final independent cold-cache acceptance remain tracked in the product status.
