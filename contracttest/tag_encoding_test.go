package contracttest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Compare native field shapes and combinations with the independent official schema.
func TestNativeTagEncodingMatrix(t *testing.T) {
	independent := official32(t)
	cases := []struct {
		name, role, value, code string
		semanticOnly            bool
	}{
		{"tag minimal", "tag", `{"name":"items"}`, "", false},
		{"tag all strings", "tag", `{"name":"items","summary":"Items","description":"Details","kind":"custom-category"}`, "", false},
		{"tag empty name", "tag", `{"name":"","kind":"","summary":""}`, "", false},
		{"tag external docs", "tag", `{"name":"items","externalDocs":{"url":"./guide","description":"Read more"}}`, "", false},
		{"tag opaque extension", "tag", `{"name":"items","x-data":{"parent":12,"$ref":"file:///not-read"}}`, "", false},
		{"tag missing name", "tag", `{}`, "tag.name", false},
		{"tag null name", "tag", `{"name":null}`, "tag.name", false},
		{"tag summary type", "tag", `{"name":"items","summary":false}`, "tag.string", false},
		{"tag description type", "tag", `{"name":"items","description":{}}`, "tag.string", false},
		{"tag parent type", "tag", `{"name":"items","parent":12}`, "tag.string", false},
		{"tag kind type", "tag", `{"name":"items","kind":[]}`, "tag.string", false},
		{"tag null", "tag", `null`, "object", false},
		{"tag nested array", "tag", `[{"name":"items"}]`, "object", false},
		{"tag unknown field", "tag", `{"name":"items","parents":[]}`, "tag.field", false},
		{"tag docs container", "tag", `{"name":"items","externalDocs":[]}`, "object", false},
		{"tag docs URL required", "tag", `{"name":"items","externalDocs":{}}`, "externalDocs.url", false},
		{"tag docs URL type", "tag", `{"name":"items","externalDocs":{"url":0}}`, "externalDocs.url", false},
		{"tag docs URL syntax", "tag", `{"name":"items","externalDocs":{"url":"a b"}}`, "externalDocs.url", true},
		{"tags empty", "tags", `[]`, "", false},
		{"tags null", "tags", `null`, "tag.array", false},
		{"tags object", "tags", `{"name":"items"}`, "tag.array", false},
		{"tag hierarchy", "tags", `[{"name":"items","parent":"resources"},{"name":"resources"}]`, "", false},
		{"tag empty parent identity", "tags", `[{"name":"child","parent":""},{"name":""}]`, "", false},
		{"tag missing parent", "tag", `{"name":"items","parent":"missing"}`, "tag.parent", true},
		{"tag empty missing parent", "tag", `{"name":"items","parent":""}`, "tag.parent", true},
		{"tag duplicate", "tags", `[{"name":"items"},{"name":"items"}]`, "tag.duplicate", true},
		{"tag cycle", "tags", `[{"name":"items","parent":"items"}]`, "tag.cycle", true},
		{"tag cycle through empty identity", "tags", `[{"name":"","parent":"items"},{"name":"items","parent":""}]`, "tag.cycle", true},
		{"media empty", "media", `{}`, "", false},
		{"media binary schema", "media", `{"schema":false,"itemSchema":true}`, "", false},
		{"media positional", "media", `{"schema":{"type":"array"},"prefixEncoding":[{}],"itemEncoding":{"contentType":"application/json"}}`, "", false},
		{"media streamed positional", "media", `{"itemSchema":true,"prefixEncoding":[],"itemEncoding":{}}`, "", false},
		{"media named", "media", `{"schema":{"type":"object","properties":{"part":{}}},"encoding":{"part":{}}}`, "", false},
		{"media description type", "media", `{"description":false}`, "media.string", false},
		{"media unknown field", "media", `{"itemSchemas":[]}`, "media.field", false},
		{"media array", "media", `[{}]`, "object", false},
		{"media dictionary array", "mediaTypes", `[]`, "object", false},
		{"prefix object", "media", `{"itemSchema":true,"prefixEncoding":{}}`, "encoding.prefix", false},
		{"prefix null", "media", `{"itemSchema":true,"prefixEncoding":null}`, "encoding.prefix", false},
		{"prefix nested array", "media", `{"itemSchema":true,"prefixEncoding":[[]]}`, "object", false},
		{"item array", "media", `{"itemSchema":true,"itemEncoding":[]}`, "object", false},
		{"named encoding array", "media", `{"encoding":[]}`, "object", false},
		{"named encoding null", "media", `{"encoding":null}`, "object", false},
		{"named and prefix conflict", "media", `{"itemSchema":true,"encoding":{},"prefixEncoding":[]}`, "encoding.conflict", false},
		{"named and item conflict", "media", `{"itemSchema":true,"encoding":{},"itemEncoding":{}}`, "encoding.conflict", false},
		{"positional without array", "media", `{"schema":{"type":"object"},"itemEncoding":{}}`, "encoding.array", true},
		{"encoding empty", "encoding", `{}`, "", false},
		{"encoding explicit booleans", "encoding", `{"style":"form","explode":false,"allowReserved":false}`, "", false},
		{"encoding style alternatives", "encoding", `{"style":"deepObject","explode":true}`, "", false},
		{"encoding media choices", "encoding", `{"contentType":"image/png, image/jpeg"}`, "", false},
		{"encoding nested", "encoding", `{"contentType":"multipart/mixed","prefixEncoding":[{"contentType":"application/json"}],"itemEncoding":{"contentType":"text/plain"}}`, "", false},
		{"encoding named extension-like part", "encoding", `{"encoding":{"x-part":{"contentType":"text/plain"}}}`, "", false},
		{"encoding opaque extension", "encoding", `{"x-any":{"contentType":false,"$ref":"file:///not-read"}}`, "", false},
		{"encoding content type", "encoding", `{"contentType":false}`, "encoding.string", false},
		{"encoding style type", "encoding", `{"style":[]}`, "encoding.style", false},
		{"encoding style invalid", "encoding", `{"style":"matrix"}`, "encoding.style", false},
		{"encoding explode null", "encoding", `{"explode":null}`, "encoding.boolean", false},
		{"encoding reserved string", "encoding", `{"allowReserved":"false"}`, "encoding.boolean", false},
		{"encoding headers array", "encoding", `{"headers":[]}`, "encoding.headers", false},
		{"encoding unknown field", "encoding", `{"itemsEncoding":{}}`, "encoding.field", false},
		{"encoding reference forbidden", "encoding", `{"$ref":"#/components/mediaTypes/Parts"}`, "encoding.field", false},
		{"encoding nested prefix container", "encoding", `{"prefixEncoding":{}}`, "encoding.prefix", false},
		{"encoding nested item container", "encoding", `{"itemEncoding":[]}`, "object", false},
		{"encoding nested conflict", "encoding", `{"encoding":{},"itemEncoding":{}}`, "encoding.conflict", false},
		{"encoding extension-like invalid part", "encoding", `{"encoding":{"x-part":{"contentType":false}}}`, "encoding.string", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := tagEncodingDocument(tc.role, tc.value)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			independentErr := independent.Validate(value)
			if tc.semanticOnly && independentErr != nil {
				t.Fatalf("official structural limit changed: %v", independentErr)
			}
			if !tc.semanticOnly && (independentErr != nil) != (tc.code != "") {
				t.Fatalf("official schema disagreement: %v", independentErr)
			}
			report := openapi.Check(raw)
			if tc.code == "" {
				if report.HasErrors() {
					t.Fatalf("valid object rejected: %+v", report)
				}
				return
			}
			for _, d := range report.Diagnostics {
				if d.Code == "openapi.spec."+tc.code && strings.HasPrefix(d.Message, "#/") && d.Fix != "" {
					return
				}
			}
			t.Fatalf("missing located %s diagnostic: %+v", tc.code, report)
		})
	}
}

// Accept ordinary array references and implicit tuples without treating item constraints as the outer shape.
func TestPositionalEncodingSchemaEvidence(t *testing.T) {
	independent := official32(t)
	for _, tc := range []struct {
		name, schema, definitions string
		valid                     bool
	}{
		{"implicit tuple", `{"prefixItems":[{"type":"string"}],"items":false}`, ``, true},
		{"implicit items", `{"items":{"type":"string"}}`, ``, true},
		{"ordinary reference", `{"$ref":"#/components/schemas/Parts"}`, `"Parts":{"type":"array"}`, true},
		{"reference alias", `{"$ref":"#/components/schemas/Alias"}`, `"Alias":{"$ref":"#/components/schemas/Parts"},"Parts":{"type":"array"}`, true},
		{"anchored array", `{"$ref":"#parts"}`, `"Parts":{"$anchor":"parts","type":"array"}`, true},
		{"array conjunction", `{"allOf":[{"type":"array"},{"minItems":1}]}`, ``, true},
		{"array alternative", `{"anyOf":[{"type":"string"},{"type":"array"}]}`, ``, true},
		{"exclusive array alternative", `{"oneOf":[{"type":"string"},{"type":"array"}]}`, ``, true},
		{"referenced object", `{"$ref":"#/components/schemas/Parts"}`, `"Parts":{"type":"object"}`, false},
		{"unused array definition", `{"$defs":{"Unused":{"type":"array"}},"type":"object"}`, ``, false},
		{"property array is not outer array", `{"type":"object","properties":{"parts":{"type":"array"}}}`, ``, false},
		{"cycle with no array evidence", `{"$ref":"#/components/schemas/Loop"}`, `"Loop":{"$ref":"#/components/schemas/Loop"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"openapi":"3.2.0","info":{"title":"Parts","version":"1"},"paths":{},"components":{"schemas":{%s},"mediaTypes":{"Parts":{"schema":%s,"itemEncoding":{}}}}}`, tc.definitions, tc.schema))
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err = independent.Validate(value); err != nil {
				t.Fatal(err)
			}
			report := openapi.Check(raw)
			if tc.valid {
				if report.HasErrors() {
					t.Fatalf("array schema rejected: %+v", report)
				}
				return
			}
			for _, d := range report.Diagnostics {
				if d.Code == "openapi.spec.encoding.array" {
					return
				}
			}
			t.Fatalf("missing positional array diagnostic: %+v", report)
		})
	}
}

// Resolve offline array schemas and nested encoding headers without interpreting extension payloads.
func TestEncodingOfflineResources(t *testing.T) {
	const raw = `{"openapi":"3.2.0","info":{"title":"Parts","version":"1"},"paths":{},"components":{"mediaTypes":{"Parts":{"schema":{"$ref":"./parts.json#parts"},"prefixEncoding":[{"contentType":"multipart/mixed","itemEncoding":{"headers":{"X-Part":{"$ref":"./headers.json#/components/headers/Part"}}}}]}}}}`
	const headers = `{"openapi":"3.2.0","info":{"title":"Headers","version":"1"},"paths":{},"components":{"headers":{"Part":{"schema":{"type":"string"}}}}}`
	options := openapi.CheckOptions{BaseURI: "https://example.test/api.json", Resources: map[string][]byte{
		"https://example.test/parts.json":   []byte(`{"$anchor":"parts","type":"array"}`),
		"https://example.test/headers.json": []byte(headers),
	}}
	if report := openapi.CheckWithOptions([]byte(raw), options); report.HasErrors() {
		t.Fatalf("offline references rejected: %+v", report)
	}
	delete(options.Resources, "https://example.test/headers.json")
	report := openapi.CheckWithOptions([]byte(raw), options)
	for _, d := range report.Diagnostics {
		if d.Code == "openapi.spec.external.denied" && strings.Contains(d.Message, "/prefixEncoding/0/itemEncoding/headers/X-Part/$ref") {
			return
		}
	}
	t.Fatalf("nested missing header resource was not diagnosed: %+v", report)
}

// Retain presence through the typed construction API and validate long, ordered hierarchies in a bounded pass.
func TestTypedTagHierarchy(t *testing.T) {
	tags := []spec.Tag{{Name: ""}, {Name: "child", Parent: spec.Set("")}}
	raw, err := json.Marshal(tags)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `[{"name":""},{"name":"child","parent":""}]` {
		t.Fatalf("tag presence lost: %s", raw)
	}
	if report := openapi.Check(tagEncodingDocument("tags", string(raw))); report.HasErrors() {
		t.Fatal(report)
	}
	for i := 0; i < 1500; i++ {
		tag := spec.Tag{Name: fmt.Sprintf("tag-%04d", i)}
		if i < 1499 {
			tag.Parent = spec.Set(fmt.Sprintf("tag-%04d", i+1))
		}
		tags = append(tags, tag)
	}
	raw, err = json.Marshal(tags)
	if err != nil {
		t.Fatal(err)
	}
	if report := openapi.CheckWithOptions(tagEncodingDocument("tags", string(raw)), openapi.CheckOptions{MaxIndexBytes: 1 << 20}); report.HasErrors() {
		t.Fatalf("bounded hierarchy rejected: %+v", report)
	}
	if err = json.Unmarshal([]byte(`{"name":"child","parent":null}`), &tags[1]); err == nil {
		t.Fatal("null parent accepted by typed model")
	}
}

// Keep malformed objects isolated inside otherwise valid native documents.
func tagEncodingDocument(role, value string) []byte {
	var body string
	switch role {
	case "tag":
		body = `"tags":[` + value + `]`
	case "tags":
		body = `"tags":` + value
	case "media":
		body = `"components":{"mediaTypes":{"Parts":` + value + `}}`
	case "mediaTypes":
		body = `"components":{"mediaTypes":` + value + `}`
	case "encoding":
		body = `"components":{"mediaTypes":{"Parts":{"itemSchema":true,"itemEncoding":` + value + `}}}`
	default:
		panic("unknown test object role")
	}
	return []byte(`{"openapi":"3.2.0","info":{"title":"Native contracts","version":"1"},"paths":{},` + body + `}`)
}

// Preserve an explicit empty parent, which refers to the valid empty tag name.
func TestTagEmptyParentRoundTrip(t *testing.T) {
	raw := []byte(`{"name":"child","parent":""}`)
	var tag spec.Tag
	if err := json.Unmarshal(raw, &tag); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(tag)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"parent":""`) {
		t.Fatalf("empty parent identity was lost: %s", encoded)
	}
}
