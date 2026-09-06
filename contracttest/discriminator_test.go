package contracttest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
)

// Context rules must reject unusable dispatch targets while accepting standard union and inheritance forms.
func TestDiscriminatorContextMatrix(t *testing.T) {
	const animals = `"Cat":{"type":"object","required":["kind"],"properties":{"kind":{"const":"cat"}}},"Dog":{"type":"object","required":["kind"],"properties":{"kind":{"const":"dog"}}},"Other":{"type":"object","properties":{"kind":{"not":{"enum":["cat","dog"]}}}}`
	cases := []struct{ name, schemas, code string }{
		{"required union", `"Choice":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Dog"}],"discriminator":{"propertyName":"kind"}}`, ""},
		{"optional union fallback", `"Choice":{"anyOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Other"}],"discriminator":{"propertyName":"kind","defaultMapping":"Other"}}`, ""},
		{"optional property without fallback", `"Choice":{"anyOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Other"}],"discriminator":{"propertyName":"kind"}}`, "discriminator.default.required"},
		{"default not in union", `"Choice":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Dog"}],"discriminator":{"propertyName":"kind","defaultMapping":"Other"}}`, "discriminator.target"},
		{"mapping not in union", `"Choice":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Dog"}],"discriminator":{"propertyName":"kind","mapping":{"other":"Other"}}}`, "discriminator.target"},
		{"no composite context", `"Choice":{"required":["kind"],"discriminator":{"propertyName":"kind"}}`, "discriminator.context"},
		{"required by allOf sibling", `"Choice":{"allOf":[{"required":["kind"]}],"oneOf":[{"type":"object"}],"discriminator":{"propertyName":"kind"}}`, ""},
		{"empty property required", `"Choice":{"required":[""],"oneOf":[{"type":"object"}],"discriminator":{"propertyName":""}}`, ""},
		{"alias candidate", `"Alias":{"$ref":"#/components/schemas/Other"},"Choice":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Alias"}],"discriminator":{"propertyName":"kind","defaultMapping":"Other"}}`, ""},
		{"inherited parent", `"Choice":{"required":["kind"],"discriminator":{"propertyName":"kind","mapping":{"child":"Child"}}},"Child":{"allOf":[{"$ref":"#/components/schemas/Choice"},{"type":"object"}]}`, ""},
		{"transitive inherited parent", `"Choice":{"required":["kind"],"discriminator":{"propertyName":"kind","mapping":{"child":"Child"}}},"Middle":{"allOf":[{"$ref":"#/components/schemas/Choice"}]},"Child":{"allOf":[{"$ref":"#/components/schemas/Middle"}]}`, ""},
		{"optional parent fallback", `"Choice":{"type":"object","discriminator":{"propertyName":"kind","defaultMapping":"Child"}},"Child":{"allOf":[{"$ref":"#/components/schemas/Choice"}]}`, ""},
		{"unrelated inheritance fallback", `"Choice":{"required":["kind"],"discriminator":{"propertyName":"kind","defaultMapping":"Other"}},"Child":{"allOf":[{"$ref":"#/components/schemas/Choice"}]}`, "discriminator.target"},
		{"optional parent missing fallback", `"Choice":{"type":"object","discriminator":{"propertyName":"kind"}},"Child":{"allOf":[{"$ref":"#/components/schemas/Choice"}]}`, "discriminator.default.required"},
		{"fallback requires missing property", `"Choice":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Other"}],"discriminator":{"propertyName":"kind","defaultMapping":"Cat"}}`, "discriminator.default.optional"},
		{"cycle with explicit required", `"Choice":{"required":["kind"],"discriminator":{"propertyName":"kind","mapping":{"child":"Child"}}},"Child":{"allOf":[{"$ref":"#/components/schemas/Choice"},{"$ref":"#/components/schemas/Child"}]}`, ""},
	}
	independent := official32(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := discriminatorDocument(animals + "," + tc.schemas)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err = independent.Validate(value); err != nil {
				t.Fatalf("fixture must satisfy the official structural schema: %v", err)
			}
			report := openapi.Check(raw)
			if tc.code == "" {
				if report.HasErrors() {
					t.Fatalf("valid context rejected: %+v", report)
				}
				return
			}
			for _, d := range report.Diagnostics {
				if d.Code == "openapi.spec."+tc.code && strings.Contains(d.Message, "/discriminator") && d.Fix != "" {
					return
				}
			}
			t.Fatalf("missing normative context diagnostic %s: %+v", tc.code, report)
		})
	}
}

// Resolve dispatch targets and inheritance through explicitly supplied offline schema resources.
func TestDiscriminatorOfflineContext(t *testing.T) {
	const base = "https://example.test/api.json"
	const animalURI = "https://example.test/models/cat.json"
	resource := []byte(`{"$id":"https://example.test/models/cat.json","$anchor":"animal","type":"object","required":["kind"],"properties":{"kind":{"const":"cat"}}}`)
	raw := discriminatorDocument(`"Choice":{"oneOf":[{"$ref":"./models/cat.json#animal"},{"$ref":"#/components/schemas/Other"}],"discriminator":{"propertyName":"kind","mapping":{"cat":"./models/cat.json#animal"},"defaultMapping":"Other"}},"Other":{"type":"object","properties":{"kind":{"not":{"const":"cat"}}}}`)
	options := openapi.CheckOptions{BaseURI: base, Resources: map[string][]byte{animalURI: resource}}
	if report := openapi.CheckWithOptions(raw, options); report.HasErrors() {
		t.Fatal(report)
	}
	if report := openapi.CheckWithOptions(raw, openapi.CheckOptions{BaseURI: base}); !report.HasErrors() {
		t.Fatal("missing offline resource was accepted")
	}
	validator, err := Compile(raw, "/components/schemas/Choice", Options{BaseURI: base, Resources: options.Resources})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []string{`{"kind":"cat"}`, `{"kind":"unknown"}`, `{}`} {
		if err = validator.JSON([]byte(sample)); err != nil {
			t.Fatalf("offline branch rejected %s: %v", sample, err)
		}
	}
	parent := discriminatorDocument(`"Parent":{"type":"object","required":["kind"],"discriminator":{"propertyName":"kind","mapping":{"child":"./child.json"}}}`)
	child := []byte(`{"$id":"https://example.test/child.json","allOf":[{"$ref":"./api.json#/components/schemas/Parent"}],"required":["childOnly"]}`)
	options.Resources = map[string][]byte{"https://example.test/child.json": child}
	if report := openapi.CheckWithOptions(parent, options); report.HasErrors() {
		t.Fatal(report)
	}
	validator, err = Compile(parent, "/components/schemas/Parent", Options{BaseURI: base, Resources: options.Resources})
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON([]byte(`{"kind":"child"}`)); err != nil {
		t.Fatalf("parent validation incorrectly searched descendant constraints: %v", err)
	}
}

// Bound semantic traversal separately from ordinary indexing even when inheritance contains cycles.
func TestDiscriminatorTraversalBudget(t *testing.T) {
	const size = 100
	schemas := map[string]any{}
	for i := 0; i < size; i++ {
		schemas[fmt.Sprintf("Node%d", i)] = map[string]any{
			"required": []string{"kind"},
			"allOf":    []any{map[string]any{"$ref": fmt.Sprintf("#/components/schemas/Node%d", (i+1)%size)}},
		}
	}
	document := map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "Cyclic dispatch", "version": "1"}, "paths": map[string]any{}, "components": map[string]any{"schemas": schemas}}
	check := func(limit int) openapi.Report {
		t.Helper()
		raw, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		return openapi.CheckWithOptions(raw, openapi.CheckOptions{MaxIndexBytes: limit})
	}
	if report := check(1 << 20); report.HasErrors() {
		t.Fatalf("ordinary graph exceeds the test budget: %+v", report)
	}
	for _, schema := range schemas {
		schema.(map[string]any)["discriminator"] = map[string]any{"propertyName": "kind"}
	}
	report := check(1 << 20)
	budget := false
	for _, diagnostic := range report.Diagnostics {
		budget = budget || diagnostic.Code == "openapi.spec.budget"
	}
	if !budget {
		t.Fatalf("cyclic semantic expansion ignored the budget: %+v", report)
	}
	if report = check(16 << 20); report.HasErrors() {
		t.Fatalf("bounded valid cycle rejected with sufficient capacity: %+v", report)
	}
}

// Validate real fixture payloads so a default branch cannot overlap or exclude its named siblings.
func TestNativePolymorphismSamples(t *testing.T) {
	raw, err := os.ReadFile("../testdata/golden/openapi32-full.json")
	if err != nil {
		t.Fatal(err)
	}
	validator, err := Compile(raw, "/components/schemas/Change", Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		json  string
		valid bool
	}{
		{`{"kind":"created"}`, true},
		{`{"kind":"deleted","id":1}`, true},
		{`{"kind":"custom"}`, true},
		{`{}`, true},
		{`{"kind":"deleted","id":"wrong"}`, false},
		{`{"kind":123}`, false},
		{`null`, false},
	} {
		if err := validator.JSON([]byte(sample.json)); (err == nil) != sample.valid {
			t.Errorf("unexpected validation for %s: %v", sample.json, err)
		}
	}
}

// Keep polymorphism samples in complete standard documents with no framework-specific models.
func discriminatorDocument(schemas string) []byte {
	return []byte(`{"openapi":"3.2.0","info":{"title":"Polymorphism","version":"1"},"paths":{},"components":{"schemas":{` + schemas + `}}}`)
}

// Discriminator hints must never make overlapping oneOf branches validate or exclude valid anyOf instances.
func TestDiscriminatorDoesNotChangeInstanceValidation(t *testing.T) {
	for _, keyword := range []string{"oneOf", "anyOf"} {
		t.Run(keyword, func(t *testing.T) {
			raw := discriminatorDocument(`"Choice":{"` + keyword + `":[{"$ref":"#/components/schemas/A"},{"$ref":"#/components/schemas/B"}],"discriminator":{"propertyName":"kind","mapping":{"A":"A","B":"B"}}},"A":{"type":"object","required":["kind"]},"B":{"type":"object","required":["kind"]}`)
			if report := openapi.Check(raw); report.HasErrors() {
				t.Fatal(report)
			}
			validator, err := Compile(raw, "/components/schemas/Choice", Options{})
			if err != nil {
				t.Fatal(err)
			}
			err = validator.JSON([]byte(`{"kind":"A"}`))
			if (err == nil) != (keyword == "anyOf") {
				t.Fatalf("discriminator changed %s result: %v", keyword, err)
			}
		})
	}
}
