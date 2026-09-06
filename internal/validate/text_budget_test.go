package validate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Retrieval addresses and caller-selected pointers cannot bypass the text budget.
func TestIndexMetadataBudget(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"}}`)
	long := "https://example.test/" + strings.Repeat("x", 4096)
	for _, options := range []Options{
		{BaseURI: long, MaxIndexBytes: 1024},
		{Resources: map[string][]byte{long: []byte(`true`)}, MaxIndexBytes: 1024},
		{ExampleResources: map[string][]byte{long: []byte(`0`)}, MaxIndexBytes: 1024},
	} {
		issues := CheckWithOptions(raw, options)
		if len(issues) != 1 || issues[0].Code != "openapi.spec.budget" {
			t.Fatal(issues)
		}
	}
	if bundle, issues := BundleSchemas([]byte(`true`), "/"+strings.Repeat("x", 4096), Options{MaxIndexBytes: 1024}); bundle != nil || len(issues) != 1 || issues[0].Code != "openapi.spec.budget" {
		t.Fatal(bundle, issues)
	}
	for _, options := range []Options{{MaxIndexBytes: -1}, {MaxNormalizedBytes: -1}} {
		if issues := CheckWithOptions(raw, options); len(issues) != 1 || issues[0].Code != "openapi.spec.options" {
			t.Fatal(issues)
		}
	}
}

// Diagnostics share the index budget for repeated long paths and remain deterministic failures.
func TestDiagnosticTextBudget(t *testing.T) {
	dependencies := map[string]any{}
	for i := 0; i < 100; i++ {
		dependencies[fmt.Sprintf("field%03d", i)] = false
	}
	raw, err := json.Marshal(map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "a", "version": "1"}, "components": map[string]any{"schemas": map[string]any{strings.Repeat("p", 2048): map[string]any{"dependentRequired": dependencies}}}})
	if err != nil {
		t.Fatal(err)
	}
	var previous string
	for i := 0; i < 5; i++ {
		issues := CheckWithOptions(raw, Options{MaxIndexBytes: 32 << 10})
		exhausted := 0
		for _, issue := range issues {
			if issue.Code == "openapi.spec.budget" {
				exhausted++
			}
		}
		if exhausted != 1 || len(issues) >= 100 {
			t.Fatal(issues)
		}
		encoded, err := json.Marshal(issues)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && previous != string(encoded) {
			t.Fatal("diagnostics are unstable")
		}
		previous = string(encoded)
	}
}

// URI resolution of valid references is charged as well as failed diagnostics.
func TestResolvedReferenceTextBudget(t *testing.T) {
	schemas := map[string]any{"value": map[string]any{"type": "string"}}
	for i := 0; i < 30; i++ {
		schemas[fmt.Sprint(i)] = map[string]any{"$ref": "#/components/schemas/value"}
	}
	raw, err := json.Marshal(map[string]any{"openapi": "3.2.0", "$self": "https://example.test/" + strings.Repeat("x", 2048), "info": map[string]any{"title": "a", "version": "1"}, "components": map[string]any{"schemas": schemas}})
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckWithOptions(raw, Options{MaxIndexBytes: 32 << 10})
	if len(issues) != 1 || issues[0].Code != "openapi.spec.budget" {
		t.Fatal(issues)
	}
	if issues := CheckWithOptions(raw, Options{MaxIndexBytes: 256 << 10}); len(issues) > 0 {
		t.Fatal(issues)
	}
}

// Normalized budgets account for JSON escaping and accept the exact encoded boundary.
func TestNormalizedJSONBoundary(t *testing.T) {
	for _, raw := range []string{`true`, `false`, `{}`, `{"description":"<>&\u2028\u2029\t\"\\"}`, `{"$defs":{"v":{"type":"string"}},"allOf":[{"$ref":"#/$defs/v"},{"$ref":"#/$defs/v"}]}`} {
		bundle, issues := BundleSchemas([]byte(raw), "", Options{})
		if len(issues) > 0 {
			t.Fatal(issues)
		}
		encoded, err := json.Marshal(bundle.Value)
		if err != nil {
			t.Fatal(err)
		}
		if _, issues := BundleSchemas([]byte(raw), "", Options{MaxNormalizedBytes: len(encoded)}); len(issues) > 0 {
			t.Fatal(raw, issues)
		}
		if result, issues := BundleSchemas([]byte(raw), "", Options{MaxNormalizedBytes: len(encoded) - 1}); result != nil || len(issues) != 1 || issues[0].Code != "openapi.spec.budget" {
			t.Fatal(result, issues)
		}
	}
}

// Cross-checks budget boundaries for JSON values against the standard encoder.
func TestJSONSizeAgainstEncoder(t *testing.T) {
	for _, value := range []any{nil, true, false, json.Number("1e100"), "\x00\x01\b\f\r\n\t\"\\<>&\u2028\u2029\u4e2d\u6587\xff", []any{}, map[string]any{}, []any{false, nil, json.Number("0")}, map[string]any{"\x00<&": []any{"a", json.Number("1.25")}}} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		exact := byteBudget{remaining: len(raw)}
		if !exact.jsonValue(value) || exact.remaining != 0 {
			t.Fatalf("value=%#v size=%d remaining=%d", value, len(raw), exact.remaining)
		}
		small := byteBudget{remaining: len(raw) - 1}
		if small.jsonValue(value) {
			t.Fatalf("out-of-range value was not rejected: %#v", value)
		}
	}
}

// Covers default JSON escaping for Unicode, invalid UTF-8 and control characters.
func FuzzJSONStringBudget(f *testing.F) {
	for _, seed := range []string{"", "a", "Example", "<>&\u2028\u2029", "\x00\xff\n\t\"\\"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 16384 {
			return
		}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		exact := byteBudget{remaining: len(raw)}
		if !exact.quoted(value) || exact.remaining != 0 {
			t.Fatalf("size=%d remaining=%d", len(raw), exact.remaining)
		}
	})
}
