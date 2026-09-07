package contracttest

import (
	"fmt"
	"testing"

	"github.com/openapi-golang/openapi"
)

// Check every defined location/style pairing rather than inferring serialization rules from Gin behavior.
func TestNativeParameterStyles(t *testing.T) {
	independent := official32(t)
	styles := []string{"matrix", "label", "simple", "form", "spaceDelimited", "pipeDelimited", "deepObject", "cookie", "unknown"}
	allowed := map[string]map[string]bool{
		"path":   {"matrix": true, "label": true, "simple": true},
		"query":  {"form": true, "spaceDelimited": true, "pipeDelimited": true, "deepObject": true},
		"header": {"simple": true},
		"cookie": {"form": true, "cookie": true},
	}
	for _, location := range []string{"path", "query", "header", "cookie"} {
		for _, style := range styles {
			t.Run(location+"/"+style, func(t *testing.T) {
				raw := httpObjectDocument("parameter", fmt.Sprintf(`{"name":"id","in":%q,"style":%q,"required":true,"schema":{"type":"object"}}`, location, style))
				value, err := decode(raw)
				if err != nil {
					t.Fatal(err)
				}
				valid := allowed[location][style]
				if err = independent.Validate(value); (err == nil) != valid {
					t.Fatalf("official style disagreement: %v", err)
				}
				code := ""
				if !valid {
					code = "parameter.style"
				}
				assertHTTPObjectReport(t, openapi.Check(raw), code)
			})
		}
	}
}

// Preserve common examples and explicit false while enforcing schema/content and field applicability.
func TestNativeParameterObjectMatrix(t *testing.T) {
	independent := official32(t)
	for _, tc := range []struct {
		name, role, value, code string
		schemaDiff              bool
	}{
		{"query empty name", "parameter", `{"name":"","in":"query","schema":true}`, "", false},
		{"querystring empty name", "parameter", `{"name":"","in":"querystring","content":{"application/json":{"schema":true}}}`, "", false},
		{"cookie empty name", "parameter", `{"name":"","in":"cookie","schema":true}`, "", false},
		{"missing name", "parameter", `{"in":"query","schema":true}`, "parameter.name", false},
		{"null name", "parameter", `{"name":null,"in":"query","schema":true}`, "parameter.name", false},
		{"bad location", "parameter", `{"name":"id","in":[],"schema":true}`, "parameter.location", false},
		{"parameter object array", "parameter", `[]`, "object", false},
		{"path missing required", "parameter", `{"name":"id","in":"path","schema":true}`, "parameter.required", false},
		{"path content requires true", "parameter", `{"name":"id","in":"path","required":false,"content":{"application/json":{}}}`, "parameter.required", true},
		{"path name braces", "parameter", `{"name":"{id}","in":"path","required":true,"schema":true}`, "parameter.name", false},
		{"query booleans", "parameter", `{"name":"id","in":"query","required":false,"deprecated":false,"explode":false,"allowReserved":false,"allowEmptyValue":false,"schema":true}`, "", false},
		{"deprecated type", "parameter", `{"name":"id","in":"query","deprecated":"false","schema":true}`, "parameter.boolean", false},
		{"required type", "parameter", `{"name":"id","in":"query","required":0,"schema":true}`, "parameter.boolean", false},
		{"explode type", "parameter", `{"name":"id","in":"query","explode":null,"schema":true}`, "parameter.boolean", false},
		{"description type", "parameter", `{"name":"id","in":"query","description":{},"schema":true}`, "parameter.string", false},
		{"allow empty nonquery", "parameter", `{"name":"id","in":"cookie","allowEmptyValue":false,"schema":true}`, "parameter.field", false},
		{"header reserved not applicable", "parameter", `{"name":"id","in":"header","allowReserved":false,"schema":true}`, "parameter.field", false},
		{"native cookie reserved not applicable", "parameter", `{"name":"id","in":"cookie","style":"cookie","allowReserved":false,"schema":true}`, "parameter.field", false},
		{"legacy cookie reserved", "parameter", `{"name":"id","in":"cookie","style":"form","allowReserved":true,"schema":true}`, "", false},
		{"content with examples", "parameter", `{"name":"id","in":"query","content":{"application/json":{}},"example":{"$ref":"file:///not-read"}}`, "", false},
		{"content with allow empty", "parameter", `{"name":"id","in":"query","content":{"application/json":{}},"allowEmptyValue":true}`, "", false},
		{"content with style", "parameter", `{"name":"id","in":"query","content":{"application/json":{}},"style":"form"}`, "parameter.serialization", false},
		{"content with explode", "parameter", `{"name":"id","in":"query","content":{"application/json":{}},"explode":false}`, "parameter.serialization", false},
		{"content with reserved", "parameter", `{"name":"id","in":"query","content":{"application/json":{}},"allowReserved":false}`, "parameter.serialization", false},
		{"content array", "parameter", `{"name":"id","in":"query","content":[]}`, "object", false},
		{"content empty", "parameter", `{"name":"id","in":"query","content":{}}`, "parameter.content", false},
		{"content multiple", "parameter", `{"name":"id","in":"query","content":{"text/plain":{},"application/json":{}}}`, "parameter.content", false},
		{"schema and content", "parameter", `{"name":"id","in":"query","schema":true,"content":{"text/plain":{}}}`, "parameter.content", false},
		{"schema and content absent", "parameter", `{"name":"id","in":"query"}`, "parameter.content", false},
		{"querystring with schema", "parameter", `{"name":"id","in":"querystring","schema":true}`, "parameter.querystring", false},
		{"unknown field", "parameter", `{"name":"id","in":"query","schema":true,"styles":[]}`, "parameter.field", false},
		{"header booleans", "header", `{"schema":true,"style":"simple","required":false,"deprecated":false,"explode":false}`, "", false},
		{"header style", "header", `{"schema":true,"style":"form"}`, "parameter.style", false},
		{"header name forbidden", "header", `{"name":"id","schema":true}`, "header.field", false},
		{"header boolean type", "header", `{"deprecated":"false","schema":true}`, "header.boolean", false},
		{"header array", "header", `[]`, "object", false},
		{"header content", "header", `{"content":{"application/json":{}},"example":null}`, "", false},
		{"reference summary type", "parameter", `{"$ref":"#/components/parameters/Value","summary":false}`, "ref.string", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := httpObjectDocument(tc.role, tc.value)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			wantOfficial := tc.code == ""
			if tc.schemaDiff {
				wantOfficial = !wantOfficial
			}
			if err = independent.Validate(value); (err == nil) != wantOfficial {
				t.Fatalf("official object disagreement: %v", err)
			}
			assertHTTPObjectReport(t, openapi.Check(raw), tc.code)
		})
	}
}
