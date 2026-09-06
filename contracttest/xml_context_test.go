package contracttest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
)

// Distinguish normative XML naming from structural acceptance and JSON-only use.
func TestXMLUseSiteNames(t *testing.T) {
	cases := []struct {
		name, media, schema, components, missing string
	}{
		{"inline object", "application/xml", `{"type":"object"}`, "", "/schema/xml/name"},
		{"inline explicit element", "application/xml", `{"type":"string","xml":{"nodeType":"element"}}`, "", "/schema/xml/name"},
		{"inline attribute", "text/xml", `{"type":"string","xml":{"nodeType":"attribute"}}`, "", "/schema/xml/name"},
		{"root array item", "application/xml", `{"type":"array","items":{"type":"string"}}`, "", "/items/xml/name"},
		{"root tuple item", "application/xml", `{"type":"array","prefixItems":[{"type":"string"}]}`, "", "/prefixItems/0/xml/name"},
		{"wrapped root array", "application/xml", `{"type":"array","xml":{"wrapped":true},"items":false}`, "", "/schema/xml/name"},
		{"array wrapper does not name items", "application/xml", `{"type":"array","xml":{"nodeType":"element","name":"list"},"items":{"type":"string"}}`, "", "/items/xml/name"},
		{"structured suffix", "application/atom+xml; charset=utf-8", `{"type":"object"}`, "", "/schema/xml/name"},
		{"media case insensitive", "Application/XML", `{"type":"string"}`, "", "/schema/xml/name"},
		{"json only", "application/json", `{"type":"object"}`, "", ""},
		{"similar non XML suffix", "application/xmlish", `{"type":"object"}`, "", ""},
		{"named object", "application/xml", `{"type":"object","xml":{"name":"document"}}`, "", ""},
		{"text ignores name", "application/xml", `{"type":"string","xml":{"nodeType":"text"}}`, "", ""},
		{"cdata ignores name", "application/xml", `{"type":"string","xml":{"nodeType":"cdata"}}`, "", ""},
		{"none ignores name", "application/xml", `{"xml":{"nodeType":"none"},"properties":{"child":{"type":"string"}}}`, "", ""},
		{"named root array item", "application/xml", `{"type":"array","items":{"type":"string","xml":{"name":"item"}}}`, "", ""},
		{"property array items", "application/xml", `{"type":"object","xml":{"name":"document"},"properties":{"animals":{"type":"array","items":{"type":"string"}}}}`, "", ""},
		{"property tuple items", "application/xml", `{"xml":{"name":"document"},"properties":{"ordered":{"type":"array","prefixItems":[{"type":"string"},{"type":"number"}]}}}`, "", ""},
		{"nested property arrays", "application/xml", `{"xml":{"name":"document"},"properties":{"matrix":{"type":"array","items":{"type":"array","items":{"type":"number"}}}}}`, "", ""},
		{"legacy property attribute", "application/xml", `{"xml":{"name":"document"},"properties":{"id":{"type":"integer","xml":{"attribute":true}}}}`, "", ""},
		{"component name", "application/xml", `{"$ref":"#/components/schemas/Person"}`, `"schemas":{"Person":{"type":"object"}}`, ""},
		{"component array not property", "application/xml", `{"$ref":"#/components/schemas/People"}`, `"schemas":{"People":{"type":"array","items":{"type":"string"}}}`, "/People/items/xml/name"},
		{"named reference wrapper", "application/xml", `{"$ref":"#/components/schemas/Person","xml":{"nodeType":"element","name":"root"}}`, `"schemas":{"Person":{"type":"object","xml":{"nodeType":"none"}}}`, ""},
		{"unnamed reference wrapper", "application/xml", `{"$ref":"#/components/schemas/Person","xml":{"nodeType":"element"}}`, `"schemas":{"Person":{"type":"object"}}`, "/schema/xml/name"},
		{"referenced defs need own name", "application/xml", `{"$ref":"#/components/schemas/Container"}`, `"schemas":{"Container":{"type":"object","$defs":{"Value":{"type":"string"}},"properties":{"value":{"$ref":"#/components/schemas/Container/$defs/Value"}}}}`, "/$defs/Value/xml/name"},
		{"unused schemas do not imply XML", "application/json", `{"type":"object"}`, `"schemas":{"List":{"type":"array","items":{"type":"string","xml":{"nodeType":"element"}}}}`, ""},
		{"assertions do not create nodes", "application/xml", `{"type":"object","xml":{"name":"document"},"not":{"type":"string"},"propertyNames":{"type":"string"},"if":{"required":["id"]},"$defs":{"unused":{"type":"string"}}}`, "", ""},
		{"branches without if are ignored", "application/xml", `{"type":"object","xml":{"name":"document"},"then":{"type":"string"},"else":{"type":"number"}}`, "", ""},
		{"explicit flattened conjunction", "application/xml", `{"xml":{"name":"document"},"allOf":[{"xml":{"nodeType":"none"},"properties":{"id":{"type":"number"}}}]}`, "", ""},
		{"inline conjunction node", "application/xml", `{"xml":{"name":"document"},"allOf":[{"type":"object"}]}`, "", "/allOf/0/xml/name"},
	}
	independent := official32(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := xmlDocument(tc.media, tc.schema, tc.components)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err = independent.Validate(value); err != nil {
				t.Fatalf("fixture must satisfy the official structural schema: %v", err)
			}
			assertXMLNameReport(t, openapi.Check(raw), tc.missing)
		})
	}
}

// Reuse media components without losing the XML media type at the content use site.
func TestXMLMediaReferences(t *testing.T) {
	for _, media := range []string{"application/json", "application/xml"} {
		t.Run(media, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"openapi":"3.2.0","info":{"title":"XML","version":"1"},"paths":{},"components":{"requestBodies":{"Input":{"content":{%q:{"$ref":"#/components/mediaTypes/Alias"}}}},"mediaTypes":{"Alias":{"$ref":"#/components/mediaTypes/Payload"},"Payload":{"schema":{"type":"object"}}}}}`, media))
			missing := ""
			if media == "application/xml" {
				missing = "/components/mediaTypes/Payload/schema/xml/name"
			}
			assertXMLNameReport(t, openapi.Check(raw), missing)
		})
	}
}

// Preserve physical component and property names across offline references and resource boundaries.
func TestXMLOfflineNames(t *testing.T) {
	const base = "https://example.test/api.json"
	for _, tc := range []struct{ name, resource, missing string }{
		{"unnamed anchored root", `{"$id":"https://example.test/model.json","$anchor":"model","type":"object"}`, "shared.json#/xml/name"},
		{"named anchored root", `{"$id":"https://example.test/model.json","$anchor":"model","type":"object","xml":{"name":"Model"}}`, ""},
		{"recursive component", `{"openapi":"3.2.0","info":{"title":"Shared","version":"1"},"paths":{},"components":{"schemas":{"Model":{"$id":"https://example.test/model.json","$anchor":"model","type":"object","properties":{"next":{"$ref":"#model"}}}}}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := xmlDocument("application/xml", `{"$ref":"./model.json#model"}`, "")
			assertXMLNameReport(t, openapi.CheckWithOptions(raw, openapi.CheckOptions{BaseURI: base, Resources: map[string][]byte{"https://example.test/shared.json": []byte(tc.resource)}}), tc.missing)
		})
	}
}

// Detect unnamed XML roots in every content-bearing object without inferring names from parameter names.
func TestXMLContentOwners(t *testing.T) {
	independent := official32(t)
	for _, tc := range []struct{ name, components, missing string }{
		{"request body", `"requestBodies":{"Input":{"content":{"application/xml":{"schema":{"type":"object"}}}}}`, "/requestBodies/Input/content/application~1xml/schema/xml/name"},
		{"parameter", `"parameters":{"Filter":{"name":"filter","in":"query","content":{"application/xml":{"schema":{"type":"object"}}}}}`, "/parameters/Filter/content/application~1xml/schema/xml/name"},
		{"header", `"headers":{"Metadata":{"content":{"application/xml":{"schema":{"type":"object"}}}}}`, "/headers/Metadata/content/application~1xml/schema/xml/name"},
		{"response", `"responses":{"Output":{"description":"ok","content":{"text/xml":{"schema":{"type":"object"}}}}}`, "/responses/Output/content/text~1xml/schema/xml/name"},
		{"media component key is not a media use", `"mediaTypes":{"application.xml":{"schema":{"type":"object"}}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := xmlDocument("application/json", `{}`, tc.components)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err = independent.Validate(value); err != nil {
				t.Fatalf("fixture must satisfy the official structural schema: %v", err)
			}
			assertXMLNameReport(t, openapi.Check(raw), tc.missing)
		})
	}
}

// Charge XML traversal separately from ordinary indexing and terminate valid recursive structures.
func TestXMLTraversalBudget(t *testing.T) {
	var schemas []string
	for i := 0; i < 100; i++ {
		schemas = append(schemas, fmt.Sprintf(`"Node%d":{"type":"object","properties":{"next":{"$ref":"#/components/schemas/Node%d"}}}`, i, (i+1)%100))
	}
	components := `"schemas":{` + strings.Join(schemas, ",") + `}`
	jsonOnly := xmlDocument("application/json", `{"$ref":"#/components/schemas/Node0"}`, components)
	xmlUse := xmlDocument("application/xml", `{"$ref":"#/components/schemas/Node0"}`, components)
	if report := openapi.CheckWithOptions(jsonOnly, openapi.CheckOptions{MaxIndexBytes: 60000}); report.HasErrors() {
		t.Fatalf("ordinary indexing exhausted the traversal test budget: %+v", report)
	}
	report := openapi.CheckWithOptions(xmlUse, openapi.CheckOptions{MaxIndexBytes: 60000})
	budgetErrors := 0
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "openapi.spec.budget" {
			budgetErrors++
		}
	}
	if budgetErrors != 1 {
		t.Fatalf("expected one XML traversal budget error, got %+v", report)
	}
	if report = openapi.CheckWithOptions(xmlUse, openapi.CheckOptions{MaxIndexBytes: 1 << 20}); report.HasErrors() {
		t.Fatalf("valid recursive XML rejected with sufficient capacity: %+v", report)
	}
}

// Keep complete fixtures small and independent of framework-specific serializers.
func xmlDocument(media, schema, components string) []byte {
	return []byte(fmt.Sprintf(`{"openapi":"3.2.0","info":{"title":"XML","version":"1"},"paths":{"/document":{"get":{"responses":{"200":{"description":"XML representation","content":{%q:{"schema":%s}}}}}}},"components":{%s}}`, media, schema, components))
}

// Require an actionable error at the actual unnamed schema, or no errors for valid controls.
func assertXMLNameReport(t *testing.T, report openapi.Report, missing string) {
	t.Helper()
	if missing == "" {
		if report.HasErrors() {
			t.Fatalf("valid XML context rejected: %+v", report)
		}
		return
	}
	for _, d := range report.Diagnostics {
		if d.Code == "openapi.spec.xml.name.required" && strings.Contains(d.Message, missing) && d.Fix != "" {
			return
		}
	}
	t.Fatalf("missing XML name diagnostic at %s: %+v", missing, report)
}
