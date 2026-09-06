# Official dialect resources

This directory retains original JSON bytes published by the OpenAPI Initiative under the upstream Apache 2.0 license in LICENSE. Only the optional contracttest package embeds them; the root runtime and Swagger UI do not.

| File | Pinned source | SHA-256 |
| --- | --- | --- |
| oas31-dialect.json | https://spec.openapis.org/oas/3.1/dialect/base | 8a0e89e365dadbebce2921ce6244340c1090e9d544c60d977e9ad6b97a61227b |
| oas31-meta.json | https://spec.openapis.org/oas/3.1/meta/base | 267a88226e64e96dfc8c89dbd7e863160c84715e0fb893ca1d9fbf9f830f1f54 |
| oas32-dialect.json | https://spec.openapis.org/oas/3.2/dialect/2025-09-17 | 4e2c989f3d1e6489d41bc1ca4ade11743278c612b82fba9144c6116c79f1c273 |
| oas32-meta.json | https://spec.openapis.org/oas/3.2/meta/2025-09-17 | a1959c0aa1f9a7ce58f2b75699be5bc5187c8a25901fe04a227f1d9419ba4e9c |

The 3.1 dialect was retrieved from its official URI and pinned by content. The 3.2 files match the fixed resources used by the independent specification tests. Updates require checking provenance, licenses, checksums, and compatibility. Preloading these resources does not enable HTTP or file loaders. OAS annotations do not automatically become instance assertions; complete OpenAPI checking uses the specification checker.
