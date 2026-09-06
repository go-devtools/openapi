package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
)

// Verify return-value frontends reuse the pipeline without a framework context.
func TestReturnValueFrontend(t *testing.T) {
	dir := t.TempDir()
	source := `// Real business input for the test frontend.
package sample

// Describe creation input.
type Request struct { Name string }
// Describe response data.
type Response struct { Name string }
// Create a user
//
// Return the created result.
func Create(req Request) (Response, error) { return Response{Name:req.Name}, nil }
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/sample\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	frontend := Frontend{Name: "test-return-v1", Match: func(f Function) bool { return f.Signature.Results().Len() == 2 },
		Entry: func(f Function) []Effect {
			return []Effect{{Kind: RequestBody, MediaType: "application/json", Payload: Value{Type: f.Signature.Params().At(0).Type()}, Source: f.Source}}
		},
		Return: func(c ReturnContext) ([]Effect, error) {
			return []Effect{{Kind: ResponseBody, Status: "201", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
		},
	}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	index := result.Bundle.Index()
	if len(index) != 1 {
		t.Fatalf("unexpected candidate count: %+v", index)
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/users", OperationKey: index[0].Key}}, openapi.Config{Title: "Users", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc.JSON()), "Create a user") || !strings.Contains(string(doc.JSON()), `"201"`) {
		t.Fatalf("comments and return values were not reused: %s", doc.JSON())
	}
	output := filepath.Join(dir, "internal", "apidoc")
	if err = result.Write(WriteOptions{Dir: output, Package: "apidoc"}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(output, "zz_openapi.gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(first), "example.com/sample\"") || strings.Contains(string(first), "openapi/compiler\"") {
		t.Fatal("generated file depends on business code or the compiler")
	}
	if err = result.Check(WriteOptions{Dir: output, Package: "apidoc"}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "sample.go"), []byte(strings.Replace(source, "Create a user", "Register user", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Check(WriteOptions{Dir: output, Package: "apidoc"}) == nil {
		t.Fatal("source changes did not make the generated output stale")
	}
}
