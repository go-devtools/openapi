# Security models and validation

`spec.SecurityScheme`, `spec.OAuthFlows`, and `spec.OAuthFlow` describe authentication contracts through public Go types. They do not install authentication middleware or prove that an API enforces a declared policy.

```go
device := spec.SecurityScheme{
    Type:              "oauth2",
    Deprecated:        spec.Set(false),
    OAuth2MetadataURL: "https://auth.example.com/.well-known/oauth-authorization-server",
    Flows: &spec.OAuthFlows{
        DeviceAuthorization: &spec.OAuthFlow{
            DeviceAuthorizationURL: "https://auth.example.com/device",
            TokenURL:               "https://auth.example.com/token",
            RefreshURL:             "https://auth.example.com/refresh",
            Scopes:                 map[string]string{"read": "Read records"},
        },
    },
}
components := spec.Components{
    SecuritySchemes: map[string]spec.RefOr[spec.SecurityScheme]{
        "Device": spec.Inline(device),
        "Bearer": spec.Inline(spec.SecurityScheme{Type: "http", Scheme: "bearer"}),
    },
}
```

`SecurityScheme.Deprecated` uses `Optional[bool]`: leave it unset to omit the field, or use `spec.Set(false)` / `spec.Set(true)` to emit an explicit value. When updating code from a pinned version where this field was a `bool`, wrap assigned values in `spec.Set`. Other objects' deprecation fields are separate APIs.

For OAuth2, `flows` is required even when `oauth2MetadataUrl` is supplied. An empty `&spec.OAuthFlows{}` is valid. Every declared flow requires a `scopes` map; use `map[string]string{}` for no available scopes, because a nil map serializes to invalid `null`.

| Flow | Required endpoint fields |
| --- | --- |
| `implicit` | `authorizationUrl` |
| `password` | `tokenUrl` |
| `clientCredentials` | `tokenUrl` |
| `authorizationCode` | `authorizationUrl`, `tokenUrl` |
| `deviceAuthorization` | `deviceAuthorizationUrl`, `tokenUrl` |

`refreshUrl` is optional for every flow. Scope descriptions must be strings. Unknown or inapplicable standard fields fail validation; use `x-` extensions for project metadata. Extension values are opaque data, including at the OAuth Flows and OAuth Flow levels. Bearer scheme comparison is case-insensitive.

`openapi.Check` and `CheckWithOptions` check URL syntax without loading endpoints. Explicit absolute authorization URLs must use HTTPS. Relative URL references remain valid and must resolve against the API server selected by the consumer. Runtime server selection and every relative-URL/TLS combination are not established by this static check.

`TestNativeSecurity32Matrix` checks the same positive and negative objects through the public checker and the pinned official OpenAPI 3.2 schema with an independent engine. TLS constraints require semantic checking beyond that schema; the pinned engine also accepts an unescaped space in a URI reference, which the core rejects. `TestNativeSecurity32Model` covers typed device configuration, empty scopes, extensions, and deprecation presence.

See the [OpenAPI 3.2 security objects](https://spec.openapis.org/oas/v3.2.0.html#security-scheme-object) and the [native capability matrix](openapi32-matrix.md). The example UI intentionally offers Bearer only. Native OAuth device-flow rendering, metadata discovery, and authorization submission are not claimed by these model tests.
