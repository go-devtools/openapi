package contracttest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Cross-check security objects through the public checker and the pinned official schema.
// 通过公开检查器和固定官方 Schema 交叉验证安全对象。
func TestNativeSecurity32Matrix(t *testing.T) {
	independent := official32(t)
	cases := []struct {
		name, scheme, code string
		semanticOnly       bool
	}{
		{"array scheme", `[{"type":"http","scheme":"bearer"}]`, "object", false},
		{"empty array scheme", `[]`, "object", false},
		{"bearer", `{"type":"http","scheme":"bearer","bearerFormat":"JWT","deprecated":false}`, "", false},
		{"case insensitive bearer", `{"type":"http","scheme":"BeArEr","bearerFormat":"JWT"}`, "", false},
		{"basic", `{"type":"http","scheme":"basic"}`, "", false},
		{"mutual TLS", `{"type":"mutualTLS","deprecated":true}`, "", false},
		{"API key header", `{"type":"apiKey","name":"X-Key","in":"header"}`, "", false},
		{"API key query", `{"type":"apiKey","name":"key","in":"query"}`, "", false},
		{"API key cookie", `{"type":"apiKey","name":"key","in":"cookie"}`, "", false},
		{"OpenID relative URL", `{"type":"openIdConnect","openIdConnectUrl":"./.well-known/openid-configuration"}`, "", false},
		{"empty OAuth flows", `{"type":"oauth2","flows":{}}`, "", false},
		{"metadata and empty flows", `{"type":"oauth2","flows":{},"oauth2MetadataUrl":"https://auth.example.test/.well-known/oauth-authorization-server"}`, "", false},
		{"relative metadata", `{"type":"oauth2","flows":{},"oauth2MetadataUrl":"./.well-known/oauth-authorization-server"}`, "", false},
		{"scheme extension data", `{"type":"http","scheme":"bearer","x-any":{"flows":null,"$ref":"file:///not-read"}}`, "", false},
		{"flow extension data", `{"type":"oauth2","flows":{"x-vendor":{"not-a-flow":true}}}`, "", false},
		{"implicit", `{"type":"oauth2","flows":{"implicit":{"authorizationUrl":"/authorize","scopes":{}}}}`, "", false},
		{"password", `{"type":"oauth2","flows":{"password":{"tokenUrl":"/token","refreshUrl":"/refresh","scopes":{"read":"Read records"}}}}`, "", false},
		{"client credentials", `{"type":"oauth2","flows":{"clientCredentials":{"tokenUrl":"/token","scopes":{}}}}`, "", false},
		{"authorization code", `{"type":"oauth2","flows":{"authorizationCode":{"authorizationUrl":"/authorize","tokenUrl":"/token","scopes":{}}}}`, "", false},
		{"device authorization", `{"type":"oauth2","deprecated":true,"flows":{"deviceAuthorization":{"deviceAuthorizationUrl":"/device","tokenUrl":"/token","refreshUrl":"/refresh","scopes":{},"x-any":[null,0,false]}}}`, "", false},
		{"unknown scheme type", `{"type":"unknown"}`, "security.type", false},
		{"API key missing name", `{"type":"apiKey","in":"header"}`, "security.apiKey", false},
		{"API key invalid location", `{"type":"apiKey","name":"key","in":"path"}`, "security.apiKey", false},
		{"HTTP missing scheme", `{"type":"http"}`, "security.http", false},
		{"description type", `{"type":"http","scheme":"bearer","description":false}`, "security.string", false},
		{"deprecation type", `{"type":"http","scheme":"bearer","deprecated":"false"}`, "security.deprecated", false},
		{"deprecation null", `{"type":"http","scheme":"bearer","deprecated":null}`, "security.deprecated", false},
		{"bearer format type", `{"type":"http","scheme":"bearer","bearerFormat":1}`, "security.string", false},
		{"bearer format on basic", `{"type":"http","scheme":"basic","bearerFormat":"JWT"}`, "security.field", false},
		{"scheme on mutual TLS", `{"type":"mutualTLS","scheme":"bearer"}`, "security.field", false},
		{"unknown scheme field", `{"type":"http","scheme":"bearer","deprected":true}`, "security.field", false},
		{"missing flows", `{"type":"oauth2"}`, "security.oauth2", false},
		{"metadata does not replace flows", `{"type":"oauth2","oauth2MetadataUrl":"https://auth.example.test/metadata"}`, "security.oauth2", false},
		{"null flows", `{"type":"oauth2","flows":null}`, "security.oauth2", false},
		{"array flows", `{"type":"oauth2","flows":[]}`, "security.oauth2", false},
		{"unknown flow", `{"type":"oauth2","flows":{"authorization_code":{"scopes":{}}}}`, "security.flow", false},
		{"null flow", `{"type":"oauth2","flows":{"deviceAuthorization":null}}`, "security.flow", false},
		{"implicit missing authorization", `{"type":"oauth2","flows":{"implicit":{"scopes":{}}}}`, "security.flow.required", false},
		{"password missing token", `{"type":"oauth2","flows":{"password":{"scopes":{}}}}`, "security.flow.required", false},
		{"client credentials missing token", `{"type":"oauth2","flows":{"clientCredentials":{"scopes":{}}}}`, "security.flow.required", false},
		{"code missing token", `{"type":"oauth2","flows":{"authorizationCode":{"authorizationUrl":"/authorize","scopes":{}}}}`, "security.flow.required", false},
		{"code missing authorization", `{"type":"oauth2","flows":{"authorizationCode":{"tokenUrl":"/token","scopes":{}}}}`, "security.flow.required", false},
		{"device missing token", `{"type":"oauth2","flows":{"deviceAuthorization":{"deviceAuthorizationUrl":"/device","scopes":{}}}}`, "security.device", false},
		{"device missing authorization", `{"type":"oauth2","flows":{"deviceAuthorization":{"tokenUrl":"/token","scopes":{}}}}`, "security.device", false},
		{"scopes missing", `{"type":"oauth2","flows":{"password":{"tokenUrl":"/token"}}}`, "security.scopes", false},
		{"scopes null", `{"type":"oauth2","flows":{"password":{"tokenUrl":"/token","scopes":null}}}`, "security.scopes", false},
		{"scopes array", `{"type":"oauth2","flows":{"password":{"tokenUrl":"/token","scopes":[]}}}`, "security.scopes", false},
		{"scope description type", `{"type":"oauth2","flows":{"password":{"tokenUrl":"/token","scopes":{"read":false}}}}`, "security.scopes", false},
		{"inapplicable flow endpoint", `{"type":"oauth2","flows":{"password":{"authorizationUrl":"/authorize","tokenUrl":"/token","scopes":{}}}}`, "security.field", false},
		{"token URL type", `{"type":"oauth2","flows":{"password":{"tokenUrl":42,"scopes":{}}}}`, "security.url", false},
		{"refresh URL type", `{"type":"oauth2","flows":{"password":{"tokenUrl":"/token","refreshUrl":null,"scopes":{}}}}`, "security.url", false},
		{"metadata URL type", `{"type":"oauth2","flows":{},"oauth2MetadataUrl":false}`, "security.url", false},
		{"metadata URL whitespace", `{"type":"oauth2","flows":{},"oauth2MetadataUrl":"https://auth.example.test/invalid path"}`, "security.url", true},
		{"OpenID URL syntax", `{"type":"openIdConnect","openIdConnectUrl":"https://auth.example.test/%zz"}`, "security.url", false},
		{"metadata TLS", `{"type":"oauth2","flows":{},"oauth2MetadataUrl":"http://auth.example.test/metadata"}`, "security.url", true},
		{"token TLS", `{"type":"oauth2","flows":{"password":{"tokenUrl":"http://auth.example.test/token","scopes":{}}}}`, "security.url", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"openapi":"3.2.0","info":{"title":"Security contracts","version":"1"},"paths":{},"components":{"securitySchemes":{"Auth":` + tc.scheme + `}}}`)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			independentErr := independent.Validate(value)
			if tc.semanticOnly && independentErr != nil {
				t.Fatalf("recorded independent validation limit changed: %v", independentErr)
			}
			if !tc.semanticOnly && (independentErr != nil) != (tc.code != "") {
				t.Fatalf("official schema disagreement: %v", independentErr)
			}
			report := openapi.Check(raw)
			if tc.code == "" {
				if report.HasErrors() {
					t.Fatalf("valid security object rejected: %+v", report)
				}
				return
			}
			for _, diagnostic := range report.Diagnostics {
				if diagnostic.Code == "openapi.spec."+tc.code && strings.HasPrefix(diagnostic.Message, "#/components/securitySchemes/Auth") && diagnostic.Fix != "" {
					return
				}
			}
			t.Fatalf("missing located diagnostic %s: %+v", tc.code, report)
		})
	}
}

// Preserve explicit deprecation and construct device authorization with public typed models.
// 保留显式弃用标记，并使用公开类型构造设备授权。
func TestNativeSecurity32Model(t *testing.T) {
	doc := spec.OpenAPI{OpenAPI: "3.2.0", Info: spec.Info{Title: "Device authorization", Version: "1"},
		Paths: map[string]*spec.PathItem{}, Security: spec.Set([]spec.SecurityRequirement{}),
		Components: &spec.Components{SecuritySchemes: map[string]spec.RefOr[spec.SecurityScheme]{
			"Device": spec.Inline(spec.SecurityScheme{Type: "oauth2", Deprecated: spec.Set(false), OAuth2MetadataURL: "https://auth.example.test/metadata",
				Flows: &spec.OAuthFlows{DeviceAuthorization: &spec.OAuthFlow{
					DeviceAuthorizationURL: "/device", TokenURL: "/token", RefreshURL: "/refresh", Scopes: map[string]string{},
					Extensions: spec.Extensions{"x-device-ui": json.RawMessage(`{"input":"code"}`)},
				}},
			}),
		}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if report := openapi.Check(raw); report.HasErrors() {
		t.Fatalf("typed model rejected: %+v", report)
	}
	var decoded spec.OpenAPI
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := json.Marshal(decoded)
	if err != nil || string(raw) != string(roundtrip) {
		t.Fatalf("typed security fields changed during roundtrip: %s -> %s (%v)", raw, roundtrip, err)
	}

	value, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := official32(t).Validate(value); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{"type":"http","scheme":"bearer","deprecated":false}`,
		`{"type":"http","scheme":"bearer","deprecated":true}`,
		`{"type":"http","scheme":"bearer"}`,
	} {
		var scheme spec.SecurityScheme
		if err := json.Unmarshal([]byte(input), &scheme); err != nil {
			t.Fatal(err)
		}
		roundtrip, err := json.Marshal(scheme)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := decode([]byte(input))
		after, _ := decode(roundtrip)
		beforeRaw, _ := json.Marshal(before)
		afterRaw, _ := json.Marshal(after)
		if string(beforeRaw) != string(afterRaw) {
			t.Errorf("security presence changed: %s -> %s", input, roundtrip)
		}
	}
}
