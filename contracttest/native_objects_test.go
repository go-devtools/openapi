package contracttest

import (
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
)

// Compare native object contracts with the public checker and the pinned official validator.
func TestNativeObjects32Matrix(t *testing.T) {
	independent := official32(t)
	cases := []struct {
		name, role, value, code string
		semanticOnly            bool
	}{
		{"example empty", "example", `{}`, "", false},
		{"example null value", "example", `{"value":null}`, "", false},
		{"example false data", "example", `{"dataValue":false}`, "", false},
		{"example zero data", "example", `{"dataValue":0}`, "", false},
		{"example empty string", "example", `{"serializedValue":""}`, "", false},
		{"example paired representations", "example", `{"dataValue":{"id":0},"serializedValue":"<item id=\"0\"/>"}`, "", false},
		{"example opaque extension", "example", `{"x-any":{"$ref":"file:///not-read","xml":null}}`, "", false},
		{"example null", "example", `null`, "object", false},
		{"example array", "example", `[]`, "object", false},
		{"example array with object", "example", `[{}]`, "object", false},
		{"examples dictionary array", "examples", `[]`, "object", false},
		{"example summary type", "example", `{"summary":1}`, "example.string", false},
		{"example description type", "example", `{"description":null}`, "example.string", false},
		{"example serialized number", "example", `{"serializedValue":123}`, "example.string", false},
		{"example serialized null", "example", `{"serializedValue":null}`, "example.string", false},
		{"example external null", "example", `{"externalValue":null}`, "ref.uri", false},
		{"example unknown field", "example", `{"serialisedValue":"x"}`, "example.field", false},
		{"example value and data", "example", `{"value":null,"dataValue":null}`, "example.conflict", false},
		{"example value and serialized", "example", `{"value":false,"serializedValue":""}`, "example.conflict", false},
		{"example serialized and external", "example", `{"serializedValue":"","externalValue":"./item.xml"}`, "example.conflict", false},
		{"discriminator", "discriminator", `{"propertyName":"kind"}`, "", false},
		{"discriminator empty property name", "discriminator", `{"propertyName":""}`, "", false},
		{"discriminator mapping", "discriminator", `{"propertyName":"kind","mapping":{"item":"Thing"}}`, "", false},
		{"discriminator default only", "discriminator", `{"propertyName":"kind","defaultMapping":"Thing"}`, "", false},
		{"discriminator extension", "discriminator", `{"propertyName":"kind","x-data":{"mapping":[]}}`, "", false},
		{"discriminator missing property", "discriminator", `{}`, "discriminator.propertyName", true},
		{"discriminator null", "discriminator", `null`, "object", false},
		{"discriminator array", "discriminator", `[]`, "object", false},
		{"discriminator property null", "discriminator", `{"propertyName":null}`, "discriminator.propertyName", false},
		{"discriminator property number", "discriminator", `{"propertyName":1}`, "discriminator.propertyName", false},
		{"discriminator mapping null", "discriminator", `{"propertyName":"kind","mapping":null}`, "discriminator.mapping", false},
		{"discriminator mapping array", "discriminator", `{"propertyName":"kind","mapping":[]}`, "discriminator.mapping", false},
		{"discriminator mapping value", "discriminator", `{"propertyName":"kind","mapping":{"item":false}}`, "ref.uri", false},
		{"discriminator default null", "discriminator", `{"propertyName":"kind","defaultMapping":null}`, "ref.uri", false},
		{"discriminator unknown field", "discriminator", `{"propertyName":"kind","default":"Thing"}`, "discriminator.field", false},
		{"xml empty", "xml", `{}`, "", false},
		{"xml element", "xml", `{"nodeType":"element"}`, "", false},
		{"xml attribute", "xml", `{"nodeType":"attribute"}`, "", false},
		{"xml text name ignored", "xml", `{"nodeType":"text","name":"ignored"}`, "", false},
		{"xml cdata", "xml", `{"nodeType":"cdata"}`, "", false},
		{"xml none", "xml", `{"nodeType":"none"}`, "", false},
		{"xml legacy false", "xml", `{"attribute":false}`, "", false},
		{"xml legacy true", "xml", `{"attribute":true}`, "", false},
		{"xml wrapped array", "arrayXML", `{"wrapped":true}`, "", false},
		{"xml unwrapped array", "arrayXML", `{"wrapped":false}`, "", false},
		{"xml namespace fragment", "xml", `{"namespace":"https://example.test/ns#item"}`, "", false},
		{"xml namespace IRI", "xml", `{"namespace":"https://例子.测试/名字#项目"}`, "", false},
		{"xml opaque extension", "xml", `{"x-any":{"nodeType":0}}`, "", false},
		{"xml null", "xml", `null`, "object", false},
		{"xml array", "xml", `[]`, "object", false},
		{"xml node type null", "xml", `{"nodeType":null}`, "xml.nodeType", false},
		{"xml node type number", "xml", `{"nodeType":123}`, "xml.nodeType", false},
		{"xml node type empty", "xml", `{"nodeType":""}`, "xml.nodeType", false},
		{"xml node type unknown", "xml", `{"nodeType":"comment"}`, "xml.nodeType", false},
		{"xml name type", "xml", `{"name":false}`, "xml.string", false},
		{"xml prefix type", "xml", `{"prefix":[]}`, "xml.string", false},
		{"xml namespace type", "xml", `{"namespace":true}`, "xml.string", false},
		{"xml namespace relative", "xml", `{"namespace":"./namespace"}`, "xml.namespace", false},
		{"xml namespace empty", "xml", `{"namespace":""}`, "xml.namespace", false},
		{"xml namespace whitespace", "xml", `{"namespace":"https://example.test/a b"}`, "xml.namespace", true},
		{"xml attribute type", "xml", `{"attribute":"false"}`, "xml.boolean", false},
		{"xml wrapped type", "arrayXML", `{"wrapped":null}`, "xml.boolean", false},
		{"xml attribute false conflict", "xml", `{"nodeType":"element","attribute":false}`, "xml.conflict", false},
		{"xml attribute true conflict", "xml", `{"nodeType":"element","attribute":true}`, "xml.conflict", false},
		{"xml wrapped false conflict", "arrayXML", `{"nodeType":"none","wrapped":false}`, "xml.conflict", false},
		{"xml wrapped true conflict", "arrayXML", `{"nodeType":"element","wrapped":true}`, "xml.conflict", false},
		{"xml wrapped nonarray", "xml", `{"wrapped":true}`, "xml.wrapped", true},
		{"xml unwrapped nonarray", "xml", `{"wrapped":false}`, "xml.wrapped", true},
		{"xml unknown field", "xml", `{"node":"element"}`, "xml.field", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := nativeObjectDocument(tc.role, tc.value)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			independentErr := independent.Validate(value)
			if tc.semanticOnly && independentErr != nil {
				t.Fatalf("recorded official validation limit changed: %v", independentErr)
			}
			if !tc.semanticOnly && (independentErr != nil) != (tc.code != "") {
				t.Fatalf("official schema disagreement: %v", independentErr)
			}
			report := openapi.Check(raw)
			if tc.code == "" {
				if report.HasErrors() {
					t.Fatalf("valid native object rejected: %+v", report)
				}
				return
			}
			for _, d := range report.Diagnostics {
				if d.Code == "openapi.spec."+tc.code && strings.HasPrefix(d.Message, "#/components/") && d.Fix != "" {
					return
				}
			}
			t.Fatalf("missing located diagnostic %s: %+v", tc.code, report)
		})
	}
}

// Place one object in a valid surrounding document without altering its JSON data.
func nativeObjectDocument(role, value string) []byte {
	components := `"schemas":{"Thing":{"type":"object"}}`
	switch role {
	case "example":
		components += `,"examples":{"Sample":` + value + `}`
	case "examples":
		components += `,"examples":` + value
	case "discriminator":
		components = `"schemas":{"Variant":{"oneOf":[{"$ref":"#/components/schemas/Thing"}],"discriminator":` + value + `},"Thing":{"type":"object","required":["kind",""]}}`
	case "xml":
		components = `"schemas":{"Thing":{"type":"string","xml":` + value + `}}`
	case "arrayXML":
		components = `"schemas":{"Thing":{"type":"array","items":{"type":"string"},"xml":` + value + `}}`
	}
	return []byte(`{"openapi":"3.2.0","info":{"title":"Native objects","version":"1"},"paths":{},"components":{` + components + `}}`)
}

// Keep logical and serialized examples paired while loading external bytes only from explicit inputs.
func TestNativeExampleRepresentations(t *testing.T) {
	raw := nativeObjectDocument("example", `{"dataValue":null,"externalValue":"./item.xml"}`)
	options := openapi.CheckOptions{BaseURI: "https://example.test/api.json", ExampleResources: map[string][]byte{"https://example.test/item.xml": []byte(`<item/>`)}}
	if report := openapi.CheckWithOptions(raw, options); report.HasErrors() {
		t.Fatalf("supplied external example rejected: %+v", report)
	}
	if report := openapi.Check(raw); !report.HasErrors() {
		t.Fatal("external example did not require explicit offline content")
	}
	value, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := official32(t).Validate(value); err != nil {
		t.Fatal(err)
	}
}
