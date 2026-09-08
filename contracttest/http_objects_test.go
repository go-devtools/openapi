package contracttest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/spec"
)

// Compare HTTP object shapes and normative combinations with the independently compiled official schema.
func TestNativeHTTPObjectMatrix(t *testing.T) {
	independent := official32(t)
	cases := []struct {
		name, role, value, code string
		schemaDiff              bool
	}{
		{"server relative", "server", `{"url":"./v1","name":"dev","description":"Development"}`, "", false},
		{"server unicode template", "server", `{"url":"https://{租户}.example.test:{端口}/路径","variables":{"租户":{"default":"demo"},"端口":{"default":"443"}}}`, "", false},
		{"server encoded delimiters", "server", `{"url":"/v1/%23%3F"}`, "", false},
		{"server empty list", "servers", `[]`, "", false},
		{"server missing URL", "server", `{}`, "server.url", false},
		{"server URL type", "server", `{"url":false}`, "server.url", false},
		{"server description type", "server", `{"url":"/","description":[]}`, "server.string", false},
		{"server name type", "server", `{"url":"/","name":false}`, "server.string", false},
		{"server container", "servers", `{}`, "server.array", false},
		{"server nested array", "server", `[]`, "object", false},
		{"server variables container", "server", `{"url":"/","variables":[]}`, "object", false},
		{"server variable object", "server", `{"url":"/","variables":{"v":[]}}`, "object", false},
		{"server variable default", "server", `{"url":"/{v}","variables":{"v":{"default":false}}}`, "serverVariable.default", false},
		{"server variable missing default", "server", `{"url":"/{v}","variables":{"v":{}}}`, "serverVariable.default", false},
		{"server enum", "server", `{"url":"/{v}","variables":{"v":{"default":"","enum":["","v1"]}}}`, "", false},
		{"server enum duplicates permitted", "server", `{"url":"/{v}","variables":{"v":{"default":"v1","enum":["v1","v1"]}}}`, "", false},
		{"server enum empty", "server", `{"url":"/{v}","variables":{"v":{"default":"v1","enum":[]}}}`, "serverVariable.enum", false},
		{"server enum type", "server", `{"url":"/{v}","variables":{"v":{"default":"v1","enum":[1]}}}`, "serverVariable.enum", false},
		{"server default outside enum", "server", `{"url":"/{v}","variables":{"v":{"default":"v3","enum":["v1","v2"]}}}`, "serverVariable.default", true},
		{"server query forbidden", "server", `{"url":"/v1?mode=demo"}`, "server.url", true},
		{"server fragment forbidden", "server", `{"url":"/v1#section"}`, "server.url", true},
		{"server malformed template", "server", `{"url":"/{v"}`, "server.template", true},
		{"server duplicate variable", "server", `{"url":"/{v}/{v}","variables":{"v":{"default":"v1"}}}`, "server.template", true},
		{"server invalid percent", "server", `{"url":"/v1/%zz"}`, "server.url", true},
		{"server unescaped space", "server", `{"url":"/v1/a b"}`, "server.url", true},
		{"server unknown field", "server", `{"url":"/","names":[]}`, "server.field", false},
		{"server opaque extension", "server", `{"url":"/","x-data":{"$ref":"file:///not-read"}}`, "", false},
		{"link valid", "link", `{"operationId":"next","description":"Next","server":{"url":"/"}}`, "", false},
		{"link reference", "link", `{"operationRef":"#/paths/~1next/get"}`, "", false},
		{"link literal parameter data", "link", `{"operationId":"next","parameters":{"filter":{"$ref":"file:///not-read"},"false":false,"nil":null},"requestBody":false}`, "", true},
		{"link description type", "link", `{"operationId":"next","description":12}`, "link.string", false},
		{"link target type", "link", `{"operationId":false}`, "link.string", false},
		{"link parameters container", "link", `{"operationId":"next","parameters":[]}`, "link.parameters", false},
		{"link server container", "link", `{"operationId":"next","server":[]}`, "object", false},
		{"link unknown field", "link", `{"operationId":"next","descriptions":[]}`, "link.field", false},
		{"link missing target", "link", `{}`, "link.target", false},
		{"link conflicting targets", "link", `{"operationId":"next","operationRef":"#/paths/~1next/get"}`, "link.target", false},
		{"operation empty", "operation", `{}`, "", false},
		{"operation explicit flags", "operation", `{"deprecated":false,"tags":["","items"],"responses":{"200":{}}}`, "", false},
		{"operation array", "operation", `[]`, "object", false},
		{"operation tags container", "operation", `{"tags":{}}`, "operation.tags", false},
		{"operation tags types", "operation", `{"tags":[false]}`, "operation.tags", false},
		{"operation deprecated type", "operation", `{"deprecated":"false"}`, "operation.boolean", false},
		{"operation unknown field", "operation", `{"response":{}}`, "operation.field", false},
		{"operation server container", "operation", `{"servers":{}}`, "server.array", false},
		{"operation parameter container", "operation", `{"parameters":{}}`, "parameter.array", false},
		{"responses empty", "operation", `{"responses":{}}`, "response.missing", false},
		{"responses array", "operation", `{"responses":[]}`, "object", false},
		{"responses only extension", "operation", `{"responses":{"x-note":true}}`, "response.missing", false},
		{"response object array", "operation", `{"responses":{"200":[]}}`, "object", false},
		{"path no operations", "path", `{}`, "", false},
		{"path server container", "path", `{"servers":{}}`, "server.array", false},
		{"path parameter container", "path", `{"parameters":{}}`, "parameter.array", false},
		{"path uppercase fixed field", "path", `{"GET":{}}`, "path.field", false},
		{"path unknown field", "path", `{"unknown":{}}`, "path.field", false},
		{"additional operations container", "path", `{"additionalOperations":[]}`, "object", false},
		{"additional operation empty token", "path", `{"additionalOperations":{"":{}}}`, "method.token", false},
		{"additional operation bad token", "path", `{"additionalOperations":{"CUSTOM METHOD":{}}}`, "method.token", false},
		{"additional operation case preserved", "path", `{"get":{},"additionalOperations":{"get":{},"Get":{},"PROPFIND":{},"x-custom":{}}}`, "", false},
		{"additional operation fixed conflict", "path", `{"additionalOperations":{"QUERY":{}}}`, "method.duplicate", false},
		{"webhook container", "webhooks", `[]`, "object", false},
		{"webhook path array", "webhooks", `{"event":[]}`, "object", false},
		{"webhook valid", "webhooks", `{"event":{"post":{}}}`, "", false},
		{"callback container", "callback", `[]`, "object", false},
		{"callback valid", "callback", `{"{$request.body#/url}":{"post":{}},"x-data":{"$ref":"file:///not-read"}}`, "", false},
		{"callback path array", "callback", `{"{$request.body#/url}":[]}`, "object", false},
		{"callback dictionary array", "operation", `{"callbacks":[]}`, "object", false},
	}
	for _, tc := range cases {
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
				t.Fatalf("official schema disagreement: %v", err)
			}
			assertHTTPObjectReport(t, openapi.Check(raw), tc.code)
		})
	}
}

// Keep each edited native object inside a complete document with a real Link target.
func httpObjectDocument(role, value string) []byte {
	paths := `"paths":{"/next":{"get":{"operationId":"next","responses":{"200":{}}}}}`
	body := ""
	switch role {
	case "server":
		body = `"servers":[` + value + `]`
	case "servers", "webhooks":
		body = `"` + role + `":` + value
	case "link", "callback", "parameter", "header":
		body = `"components":{"` + map[string]string{"link": "links", "callback": "callbacks", "parameter": "parameters", "header": "headers"}[role] + `":{"Value":` + value + `}}`
	case "operation":
		paths = `"paths":{"/next":{"get":` + value + `}}`
	case "path":
		paths = `"paths":{"/next":` + value + `}`
	default:
		panic("unknown HTTP object role")
	}
	if body != "" {
		paths += "," + body
	}
	return []byte(`{"openapi":"3.2.0","info":{"title":"HTTP contracts","version":"1"},` + paths + `}`)
}

// Require actionable public errors while allowing valid native objects without warnings masquerading as errors.
func assertHTTPObjectReport(t *testing.T, report openapi.Report, code string) {
	t.Helper()
	if code == "" {
		if report.HasErrors() {
			t.Fatalf("valid native object rejected: %+v", report)
		}
		return
	}
	for _, d := range report.Diagnostics {
		if d.Code == "openapi.spec."+code && strings.HasPrefix(d.Message, "#/") && d.Fix != "" {
			return
		}
	}
	t.Fatalf("missing located %s diagnostic: %+v", code, report)
}

// Preserve required empty names when constructing native query parameters through the public model.
func TestEmptyParameterNameRoundTrip(t *testing.T) {
	parameter := spec.Parameter{In: "query", Schema: spec.Boolean(true)}
	raw, err := json.Marshal(parameter)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if name, ok := fields["name"]; !ok || name != "" {
		t.Fatalf("required empty name lost: %s", raw)
	}
	assertHTTPObjectReport(t, openapi.Check(httpObjectDocument("parameter", string(raw))), "")
}
