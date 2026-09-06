package validate

import (
	"os"
	"testing"
)

// Test OAS 3.2 semantic errors and default external-reference denial.
func TestSemanticErrors(t *testing.T) {
	for _, doc := range []string{
		`{"openapi":"3.1.0","info":{"title":"a","version":"1"},"paths":{}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"$ref":"file:///etc/passwd"}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{"/x":{"additionalOperations":{"GET":{"responses":{"200":{}}}}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"tags":[{"name":"a","parent":"b"},{"name":"b","parent":"a"}]}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"type":"unknown"}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"type":"string","minLength":-1}}}}`,
	} {
		if len(Check([]byte(doc))) == 0 {
			t.Errorf("incorrectly accepted %s", doc)
		}
	}
}

// Test boolean Schemas, querystring parameters, and reusable media types.
func TestNative32(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{"/x":{"query":{"parameters":[{"name":"query","in":"querystring","content":{"application/json":{"schema":true}}}],"responses":{"200":{"content":{"application/x-ndjson":{"$ref":"#/components/mediaTypes/Rows"}}}}}}},"components":{"mediaTypes":{"Rows":{"itemSchema":{"type":"object"}}}}}`)
	if issues := Check(raw); len(issues) != 0 {
		t.Fatalf("valid document was rejected: %+v", issues)
	}
}

// Cover Link parameter data and new OAS 3.2 object contexts in one complete standard example.
func TestFullNative32Fixture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/golden/openapi32-full.json")
	if err != nil {
		t.Fatal(err)
	}
	if issues := Check(raw); len(issues) != 0 {
		t.Fatalf("full fixture was rejected: %+v", issues)
	}
}

// Require names for querystring parameters while treating Link parameters as expression data.
func TestQuerystringNameRequired(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"parameters":{"Q":{"in":"querystring","content":{"application/json":{"schema":true}}}}}}`)
	for _, issue := range Check(raw) {
		if issue.Code == "openapi.spec.parameter.name" {
			return
		}
	}
	t.Fatal("unnamed querystring parameter must be rejected")
}
