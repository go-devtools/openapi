package consumer

import (
	"context"
	"encoding/json"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
	. "github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/contracttest"
	"github.com/go-devtools/openapi/spec"
)

// Compile a real neutral entry point whose field-read effects depend on no HTTP framework.
func requestFieldDocument(t *testing.T, entry func(Function) []Effect) (*openapi.Document, error) {
	t.Helper()
	dir := t.TempDir()
	for name, raw := range map[string]string{"go.mod": "module example.test/fields\n\ngo 1.27.1\n", "app.go": "package fields\nfunc Handle(flag bool) string { return \"ok\" }\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{{Name: "neutral-fields-v1", Match: func(f Function) bool { return f.Object.Name() == "Handle" }, Entry: entry, Return: func(c ReturnContext) ([]Effect, error) {
		return []Effect{{Kind: ResponseBody, Status: "200", MediaType: "text/plain", WireSchema: spec.Typed("string"), Source: c.Source}}, nil
	}}}})
	if err != nil {
		return nil, err
	}
	return openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/fields", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Fields", Version: "1"})
}

// Conjoin fields read along one path rather than alternatives that lose other field constraints.
func TestRequestFieldsComposeOneBody(t *testing.T) {
	doc, err := requestFieldDocument(t, func(f Function) []Effect {
		label := spec.Typed("string")
		label.MinLength = spec.Set(uint64(2))
		return []Effect{
			{Kind: EffectKind("requestField"), Name: "label", MediaType: "application/json", WireSchema: label, Required: true, Source: f.Source},
			{Kind: EffectKind("requestField"), Name: "count", MediaType: "application/json", Payload: Value{Type: types.Typ[types.Int]}, Source: f.Source},
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(doc.JSON(), "/paths/~1fields/post/requestBody/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON([]byte(`{"label":"Ada","count":3}`)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"label":"A","count":3}`, `{"label":"Ada","count":"wrong"}`, `{"count":3}`} {
		if validator.JSON([]byte(bad)) == nil {
			t.Fatalf("field constraint was lost: %s", bad)
		}
	}
	var document spec.OpenAPI
	if err = json.Unmarshal(doc.JSON(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Paths["/fields"].Post.RequestBody.Value.Required.Value {
		t.Fatal("field presence incorrectly requires the entire body")
	}
}

// Reject conflicting repeated fields instead of silently retaining the first representation.
func TestRequestFieldsRejectConflicts(t *testing.T) {
	_, err := requestFieldDocument(t, func(f Function) []Effect {
		return []Effect{{Kind: EffectKind("requestField"), Name: "item", MediaType: "multipart/form-data", WireSchema: spec.Typed("string"), Source: f.Source}, {Kind: EffectKind("requestField"), Name: "item", MediaType: "multipart/form-data", WireSchema: spec.Typed("integer"), Source: f.Source}}
	})
	if err == nil || !strings.Contains(err.Error(), "request field item has inconsistent wire representation") {
		t.Fatalf("conflicting fields were not diagnosed: %v", err)
	}
}

// Whole-object and single-field reads constrain the same body without anyOf weakening either read.
func TestRequestFieldsAndWholeBody(t *testing.T) {
	doc, err := requestFieldDocument(t, func(f Function) []Effect {
		body := spec.Typed("object")
		body.Properties = map[string]*spec.Schema{"count": spec.Typed("integer")}
		label := spec.Typed("string")
		label.MinLength = spec.Set(uint64(2))
		return []Effect{{Kind: RequestBody, MediaType: "application/json", WireSchema: body, Source: f.Source}, {Kind: RequestField, Name: "label", MediaType: "application/json", WireSchema: label, Source: f.Source}}
	})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(doc.JSON(), "/paths/~1fields/post/requestBody/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON([]byte(`{"label":"Ada","count":3}`)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"label":"A","count":3}`, `{"label":"Ada","count":"wrong"}`} {
		if validator.JSON([]byte(bad)) == nil {
			t.Fatalf("whole-body/field intersection was lost: %s", bad)
		}
	}
}

// A reachable path that reads no body prevents another path from making the body universally required.
func TestRequestBodyPresenceAcrossPaths(t *testing.T) {
	for _, source := range []string{`if flag { Require() }; return "ok"`, `if flag { return "ok" }; Require(); return "ok"`} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/presence\n\ngo 1.27.1\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package presence\nfunc Require(){}\nfunc Handle(flag bool)string{"+source+"}\n"), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{{Name: "body-presence-v1", Match: func(f Function) bool { return f.Object.Name() == "Handle" }, Call: func(c CallContext) ([]Effect, error) {
			if c.Object != nil && c.Object.Pkg() != nil && c.Object.Pkg().Path() == "example.test/presence" && c.Object.Name() == "Require" {
				return []Effect{{Kind: RequestBody, MediaType: "application/json", Required: true, WireSchema: spec.Typed("object"), Source: c.Source}}, nil
			}
			return nil, nil
		}, Return: func(c ReturnContext) ([]Effect, error) {
			return []Effect{{Kind: ResponseBody, Status: "200", MediaType: "text/plain", WireSchema: spec.Typed("string"), Source: c.Source}}, nil
		}}}})
		if err != nil {
			t.Fatal(err)
		}
		operation := result.Bundle.Index()[0].Operation
		if operation.RequestBody == nil || operation.RequestBody.Value.Required.Value {
			t.Fatal("body presence depends on path visitation order")
		}
	}
}
