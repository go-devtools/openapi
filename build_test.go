package openapi

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/go-devtools/openapi/spec"
)

// Build a neutral template without an HTTP framework.
func testBundle(t *testing.T) Bundle {
	t.Helper()
	b, err := NewBundle(BundleData{FormatVersion: 1, SpecVersion: "3.2.0", Templates: []Template{{Key: "user", Symbol: "example.com/app.User", Operation: spec.Operation{Summary: "Read user", Responses: map[string]spec.RefOr[spec.Response]{"200": spec.Inline(spec.Response{Description: "Success", Content: map[string]spec.RefOr[spec.MediaType]{"application/json": spec.Inline(spec.MediaType{Schema: &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/User"}}})}})}}}}, Components: spec.Components{Schemas: map[string]*spec.Schema{"User": spec.Typed("object"), "Unused": spec.Typed("string")}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Verify defensive copies isolate repeated document builds.
func TestBuildImmutableAndStable(t *testing.T) {
	b := testBundle(t)
	routes := []Route{{Method: "GET", Path: "/users/{id}", OperationKey: "user"}}
	doc, err := Build(b, routes, Config{Title: "User service", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	original := string(doc.JSON())
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(doc.JSON(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["security"]; exists {
		t.Fatal("security must be omitted when it is not configured")
	}
	empty, err := Build(b, routes, Config{Title: "User service", Version: "1", Security: spec.Set([]spec.SecurityRequirement{})})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(empty.JSON(), &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["security"]) != "[]" {
		t.Fatal("explicit empty security requirements must remain an empty array")
	}
	if _, err = Build(b, routes, Config{Title: "User service", Version: "1", Security: spec.Set([]spec.SecurityRequirement(nil))}); err == nil {
		t.Fatal("security must not be null")
	}
	data := doc.JSON()
	data[0] = '!'
	if string(doc.JSON()) != original {
		t.Fatal("caller modified shared JSON")
	}
	snapshot := b.Snapshot()
	snapshot.Templates[0].Operation.Summary = "Tampered"
	if b.Snapshot().Templates[0].Operation.Summary != "Read user" {
		t.Fatal("snapshot shares mutable state")
	}
	if strings.Contains(original, "Unused") {
		t.Fatal("unreachable components were not pruned")
	}
	var parsed spec.OpenAPI
	if err = json.Unmarshal(doc.JSON(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Paths["/users/{id}"].Get.Parameters) != 1 {
		t.Fatal("standard path parameters were not linked")
	}
	reversed := []Route{routes[0]}
	again, err := Build(b, reversed, Config{Title: "User service", Version: "1"})
	if err != nil || string(again.JSON()) != original {
		t.Fatalf("generation is unstable: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, e := Build(b, routes, Config{Title: "User service", Version: "1"})
			if e != nil || string(d.JSON()) != original {
				t.Error("concurrent linking produced inconsistent results")
			}
		}()
	}
	wg.Wait()
}

// Reject unknown templates, duplicate routes, and unresolved facts.
func TestBuildRejectsUnresolvedAndConflicts(t *testing.T) {
	b := testBundle(t)
	for _, routes := range [][]Route{{{Method: "GET", Path: "/a", OperationKey: "missing"}}, {{Method: "GET", Path: "/a", OperationKey: "user"}, {Method: "GET", Path: "/a", OperationKey: "user"}}, {{Method: "GET", Path: "/users/{id", OperationKey: "user"}}} {
		if _, err := Build(b, routes, Config{Title: "Service", Version: "1"}); err == nil {
			t.Fatalf("route was incorrectly accepted: %+v", routes)
		}
	}
	data := b.Snapshot()
	data.Templates[0].Diagnostics = []Diagnostic{{Code: "frontend.unknown", Severity: Error, Message: "Unknown response", Fix: "Register a centralized rule"}}
	b, err := NewBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Build(b, []Route{{Method: "GET", Path: "/a", OperationKey: "user"}}, Config{Title: "Service", Version: "1"}); err == nil {
		t.Fatal("unresolved facts were hidden")
	}
	if _, err = Build(b, nil, Config{Title: "Service", Version: "1"}); err != nil {
		t.Fatal("unselected candidate polluted the document", err)
	}
}

// Reject future formats and unknown required capabilities at the read boundary.
func TestBundleCompatibility(t *testing.T) {
	data := testBundle(t).Snapshot()
	data.FormatVersion = 2
	if _, err := NewBundle(data); err == nil {
		t.Fatal("unknown format was accepted")
	}
	data.FormatVersion = 1
	data.Capabilities = []string{"future.required"}
	if _, err := NewBundle(data); err == nil {
		t.Fatal("unknown capability was accepted")
	}
}
