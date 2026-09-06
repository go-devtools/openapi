package compiler

import (
	"context"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Verify mutually exclusive conditions on consecutive calls intersect during compilation rather than runtime analysis.
// 验证连续调用的互斥请求条件在源码分析阶段求交，而不是在运行时重跑分析器。
func TestCompileRequestConditions(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
type Channel struct{}
func (*Channel) First() {}
func (*Channel) Second() {}
func (*Channel) Write() {}
func H(c *Channel){c.First();c.Second();c.Write()}
`
	for name, body := range map[string]string{"go.mod": "module example.test/conditional\n\ngo 1.27.1\n", "app.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	front := Frontend{Name: "neutral-conditional-v1", Match: func(f Function) bool { return f.Object.Name() == "H" }, CallOutcomes: func(c CallContext) ([]CallOutcome, error) {
		if c.Object == nil || c.Object.Name() == "Write" {
			return nil, nil
		}
		first := CallOutcome{When: openapi.RequestCondition{Methods: []string{"GET"}}}
		second := CallOutcome{When: openapi.RequestCondition{ExceptMethods: []string{"GET"}, MediaTypes: []string{"application/json"}}}
		if c.Object.Name() == "First" {
			first.Effects = []Effect{{Kind: ResponseCommit, Status: "201", Source: c.Source}}
			second.Effects = []Effect{{Kind: ResponseCommit, Status: "202", Source: c.Source}}
		} else {
			first.Effects = []Effect{{Kind: Handled, Source: c.Source}}
			second.Effects = []Effect{{Kind: Unresolved, Message: "unsupported branch", Source: c.Source}}
		}
		return []CallOutcome{first, second}, nil
	}, Call: func(c CallContext) ([]Effect, error) {
		if c.Object != nil && c.Object.Name() == "Write" {
			return []Effect{{Kind: ResponseBody, Status: "200", MediaType: "text/plain", WireSchema: spec.Typed("string"), Payload: Value{Type: types.Typ[types.String]}, Source: c.Source}}, nil
		}
		return nil, nil
	}}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{front}})
	if err != nil {
		t.Fatal(err)
	}
	key := result.Bundle.Index()[0].Key
	document, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/value", OperationKey: key}}, openapi.Config{Title: "Conditions", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(document.JSON()), `"202"`) || !strings.Contains(string(document.JSON()), `"201"`) {
		t.Fatal("impossible call combinations leaked")
	}
	if !strings.Contains(string(result.Bundle.JSON()), `"variants"`) {
		t.Fatal("request conditions were discarded")
	}
	found := false
	for _, fact := range document.Report().Facts {
		found = found || fact.When != nil
	}
	if !found {
		t.Fatal("condition provenance was discarded")
	}
	if _, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/value", OperationKey: key, RequestMediaTypes: []string{"application/json"}}}, openapi.Config{Title: "Conditions", Version: "1"}); err == nil {
		t.Fatal("selected branch diagnostic was discarded")
	}
}
