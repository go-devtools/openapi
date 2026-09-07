package contracttest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Cross-check metadata presence, types and identifiers against the pinned independent schema.
func TestNativeMetadataMatrix(t *testing.T) {
	independent := official32(t)
	cases := []struct {
		name, role, value, code string
		schemaDiff              bool
	}{
		{"root unknown field", "extra", `{"description":"typo"}`, "root.field", false},
		{"root opaque extension", "extra", `{"x-data":{"$ref":"file:///not-read"}}`, "", false},
		{"root no structure", "root", `{"openapi":"3.2.0","info":{"title":"","version":""}}`, "root.required", false},
		{"root components only", "root", `{"openapi":"3.2.0","info":{"title":"","version":""},"components":{}}`, "", false},
		{"root webhooks only", "root", `{"openapi":"3.2.0","info":{"title":"","version":""},"webhooks":{}}`, "", false},
		{"dialect type", "extra", `{"jsonSchemaDialect":false}`, "root.string", false},
		{"dialect relative", "extra", `{"jsonSchemaDialect":"./dialect"}`, "", true},
		{"dialect bad URI", "extra", `{"jsonSchemaDialect":"https://example.test/a b"}`, "root.uri", false},
		{"info empty strings", "info", `{"title":"","version":"","summary":"","description":""}`, "", false},
		{"info missing title", "info", `{"version":"1"}`, "info.required", false},
		{"info missing version", "info", `{"title":"API"}`, "info.required", false},
		{"info title type", "info", `{"title":null,"version":"1"}`, "info.string", false},
		{"info version type", "info", `{"title":"API","version":false}`, "info.string", false},
		{"info summary type", "info", `{"title":"API","version":"1","summary":false}`, "info.string", false},
		{"info description type", "info", `{"title":"API","version":"1","description":{}}`, "info.string", false},
		{"info terms type", "info", `{"title":"API","version":"1","termsOfService":[]}`, "info.string", false},
		{"info relative terms", "info", `{"title":"API","version":"1","termsOfService":"../terms#usage"}`, "", false},
		{"info unknown field", "info", `{"title":"API","version":"1","versions":"typo"}`, "info.field", false},
		{"contact empty", "contact", `{}`, "", false},
		{"contact array", "contact", `[]`, "object", false},
		{"contact name type", "contact", `{"name":false}`, "contact.string", false},
		{"contact URL type", "contact", `{"url":false}`, "contact.string", false},
		{"contact email type", "contact", `{"email":false}`, "contact.string", false},
		{"contact relative URL", "contact", `{"url":"../support","name":""}`, "", false},
		{"contact unknown field", "contact", `{"emails":[]}`, "contact.field", false},
		{"contact mailbox", "contact", `{"email":"support+api@example.test"}`, "", false},
		{"contact quoted mailbox", "contact", `{"email":"\"api team\"@example.test"}`, "", false},
		{"contact IPv4 mailbox", "contact", `{"email":"api@[127.0.0.1]"}`, "", false},
		{"contact IPv6 mailbox", "contact", `{"email":"api@[IPv6:2001:db8::1]"}`, "", false},
		{"contact missing at", "contact", `{"email":"invalid"}`, "contact.email", false},
		{"contact empty email", "contact", `{"email":""}`, "contact.email", false},
		{"contact empty local", "contact", `{"email":"@example.test"}`, "contact.email", true},
		{"contact display name", "contact", `{"email":"Team <api@example.test>"}`, "contact.email", false},
		{"contact angle mailbox", "contact", `{"email":"<api@example.test>"}`, "contact.email", false},
		{"contact multiple mailbox", "contact", `{"email":"a@example.test,b@example.test"}`, "contact.email", false},
		{"contact leading dot", "contact", `{"email":".api@example.test"}`, "contact.email", false},
		{"contact repeated dot", "contact", `{"email":"a..b@example.test"}`, "contact.email", false},
		{"contact domain label", "contact", `{"email":"api@-example.test"}`, "contact.email", false},
		{"contact domain whitespace", "contact", `{"email":"api@exam ple.test"}`, "contact.email", false},
		{"license missing name", "license", `{}`, "license.required", false},
		{"license empty name", "license", `{"name":""}`, "", false},
		{"license type", "license", `{"name":false}`, "license.string", false},
		{"license identifier type", "license", `{"name":"MIT","identifier":false}`, "license.string", false},
		{"license SPDX expression", "license", `{"name":"Dual license","identifier":"MIT OR Apache-2.0"}`, "", false},
		{"license relative URL", "license", `{"name":"Custom","url":"../LICENSE"}`, "", false},
		{"license conflict", "license", `{"name":"MIT","identifier":"MIT","url":"../LICENSE"}`, "license.conflict", false},
		{"license empty conflict", "license", `{"name":"MIT","identifier":"","url":""}`, "license.conflict", false},
		{"license unknown field", "license", `{"name":"MIT","identifiers":[]}`, "license.field", false},
		{"components unknown field", "components", `{"wrongSchemas":{}}`, "components.field", false},
		{"components extension", "components", `{"x-data":{"$ref":"file:///not-read"}}`, "", false},
		{"schema property names unrestricted", "components", `{"schemas":{"Model":{"properties":{"a/b":true,"角色":false},"$defs":{"a/b":true}}}}`, "", false},
		{"request required false", "requestBody", `{"content":{"application/json":{}},"required":false}`, "", false},
		{"request required type", "requestBody", `{"content":{"application/json":{}},"required":"false"}`, "requestBody.boolean", false},
		{"request description type", "requestBody", `{"content":{"application/json":{}},"description":null}`, "requestBody.string", false},
		{"request unknown field", "requestBody", `{"content":{"application/json":{}},"contents":{}}`, "requestBody.field", false},
		{"request content missing", "requestBody", `{}`, "request.content", false},
		{"request content array", "requestBody", `{"content":[]}`, "request.content", false},
		{"request empty content policy", "requestBody", `{"content":{}}`, "request.content", true},
		{"response empty", "response", `{}`, "", false},
		{"response descriptions", "response", `{"summary":"","description":""}`, "", false},
		{"response unknown field", "response", `{"summaries":[]}`, "response.field", false},
		{"response summary type", "response", `{"summary":false}`, "response.summary", false},
		{"response headers type", "response", `{"headers":[]}`, "object", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := metadataDocument(t, tc.role, tc.value)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			expected := tc.code == ""
			if tc.schemaDiff {
				expected = !expected
			}
			if err = independent.Validate(value); (err == nil) != expected {
				t.Fatalf("independent disagreement: %v", err)
			}
			assertMetadataReport(t, openapi.Check(raw), tc.code)
		})
	}
}

// Embed one native object without mixing metadata into Schema properties or literal data.
func metadataDocument(t *testing.T, role, text string) []byte {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "Metadata", "version": "1"}, "paths": map[string]any{}}
	switch role {
	case "root":
		doc = value.(map[string]any)
	case "extra":
		for k, v := range value.(map[string]any) {
			doc[k] = v
		}
	case "info", "components":
		doc[role] = value
	case "contact", "license":
		doc["info"].(map[string]any)[role] = value
	case "requestBody", "response":
		doc["components"] = map[string]any{map[string]string{"requestBody": "requestBodies", "response": "responses"}[role]: map[string]any{"Value": value}}
	default:
		t.Fatalf("unknown object role: %s", role)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Require a stable located diagnostic through the public API, including root-level errors.
func assertMetadataReport(t *testing.T, report openapi.Report, code string) {
	t.Helper()
	if code == "" {
		if report.HasErrors() {
			t.Fatalf("valid metadata rejected: %+v", report)
		}
		return
	}
	for _, issue := range report.Diagnostics {
		if issue.Code == "openapi.spec."+code && strings.Contains(issue.Message, "#") && issue.Fix != "" {
			return
		}
	}
	t.Fatalf("missing %s: %+v", code, report)
}

// Apply component-name restrictions to every component family without restricting nested map keys.
func TestNativeComponentNames(t *testing.T) {
	independent := official32(t)
	for kind, value := range map[string]string{"schemas": "true", "responses": "{}", "parameters": `{"name":"","in":"query","schema":true}`, "examples": "{}", "requestBodies": `{"content":{"application/json":{}}}`, "headers": `{"schema":true}`, "securitySchemes": `{"type":"http","scheme":"bearer"}`, "links": `{"operationRef":"#/paths/~1target/get"}`, "callbacks": "{}", "pathItems": "{}", "mediaTypes": "{}"} {
		for _, name := range []string{"A.z-Z_09", "x-component", "", "a/b", "中文"} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				nameJSON, _ := json.Marshal(name)
				raw := []byte(fmt.Sprintf(`{"openapi":"3.2.0","info":{"title":"Names","version":"1"},"paths":{"/target":{"get":{}}},"components":{"%s":{%s:%s}}}`, kind, nameJSON, value))
				valid := name == "A.z-Z_09" || name == "x-component"
				doc, err := decode(raw)
				if err != nil {
					t.Fatal(err)
				}
				if err = independent.Validate(doc); (err == nil) != valid {
					t.Fatalf("independent disagreement: %v", err)
				}
				code := ""
				if !valid {
					code = "components.name"
				}
				assertMetadataReport(t, openapi.Check(raw), code)
			})
		}
	}
}

// Preserve required empty metadata strings in native public-model serialization.
func TestNativeEmptyMetadataRoundTrip(t *testing.T) {
	raw, err := json.Marshal(spec.OpenAPI{OpenAPI: "3.2.0", Info: spec.Info{License: &spec.License{}}, Components: &spec.Components{}})
	if err != nil {
		t.Fatal(err)
	}
	assertMetadataReport(t, openapi.Check(raw), "")
}

// Check metadata references without contacting any target or relaxing percent-escape syntax.
func TestNativeMetadataURIs(t *testing.T) {
	independent := official32(t)
	for _, role := range []string{"info", "contact", "license"} {
		for _, uri := range []string{"", "../terms#section", "https://example.test/p?q=%20", "urn:example:license", "file:///unread/license", "https://example.test/a b", "https://example.test/?bad=%Q1", "https://example.test/#bad%", "https://[invalid]/"} {
			t.Run(role+"/"+uri, func(t *testing.T) {
				field := "url"
				object := map[string]any{}
				if role == "info" {
					field = "termsOfService"
					object["title"] = "API"
					object["version"] = "1"
				}
				if role == "license" {
					object["name"] = "Custom"
				}
				object[field] = uri
				data, err := json.Marshal(object)
				if err != nil {
					t.Fatal(err)
				}
				raw := metadataDocument(t, role, string(data))
				doc, err := decode(raw)
				if err != nil {
					t.Fatal(err)
				}
				valid := !strings.Contains(uri, " ") && !strings.Contains(uri, "%Q") && !strings.HasSuffix(uri, "%") && !strings.Contains(uri, "[invalid]")
				// The independent URI parser accepts raw spaces and malformed query escapes.
				independentValid := valid || strings.Contains(uri, " ") || strings.Contains(uri, "%Q")
				if err = independent.Validate(doc); (err == nil) != independentValid {
					t.Fatalf("independent URI disagreement: %v", err)
				}
				code := ""
				if !valid {
					code = role + ".uri"
				}
				assertMetadataReport(t, openapi.Check(raw), code)
			})
		}
	}
}

// Validate metadata in complete explicitly supplied resources before resolving operation references.
func TestNativeMetadataOfflineResource(t *testing.T) {
	raw := metadataDocument(t, "extra", `{}`)
	resource := metadataDocument(t, "contact", `{"email":"missing-at"}`)
	report := openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{"https://metadata.test/shared.json": resource}})
	assertMetadataReport(t, report, "contact.email")
	for _, issue := range report.Diagnostics {
		if issue.Code == "openapi.spec.contact.email" && strings.Contains(issue.Message, "shared.json#/info/contact/email") {
			return
		}
	}
	t.Fatalf("resource location missing: %+v", report)
}
