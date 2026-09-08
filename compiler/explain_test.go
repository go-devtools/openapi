package compiler_test

import (
	"context"
	"encoding/json"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/spec"
)

// Compile real source with a return-value frontend and no framework dependency.
func explainFixture(t *testing.T) (string, compiler.Options) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/explain\n\ngo 1.27.1\n",
		"dto/dto.go": `package dto
// Shared identity.
type Identity struct {
 // User name.
 // @openapi minLength=3
 Name string
}
// A typed page.
type Page[T any] struct {
 // Returned value.
 Value T
}
`,
		"app.go": `package app
import "example.com/explain/dto"
// A user result.
type User struct {
 dto.Identity
 // Hidden from JSON.
 Hidden string ` + "`json:\"-\"`" + `
}
// Not used by an endpoint.
type Unused struct { Value string }
// Create a user.
// @openapi response status=400
func Create() dto.Page[User] { return dto.Page[User]{Value: User{}} }
`,
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	front := compiler.Frontend{Name: "explain-return-v1", Match: func(f compiler.Function) bool { return f.Object.Name() == "Create" }, Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		source := c.Source
		source.Kind = "derived"
		source.Rule = "test.return"
		return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "201", MediaType: "application/json", Payload: c.Values[0], Source: source}}, nil
	}}
	return dir, compiler.Options{Load: compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []compiler.Frontend{front}, Explain: true}
}

// Explain imported and generic origins while preserving declared and derived evidence.
func TestExplainFieldsAndResponses(t *testing.T) {
	_, options := explainFixture(t)
	result, err := compiler.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	field, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/explain/dto.Identity.Name"})
	if err != nil {
		t.Fatal(err)
	}
	if field.Kind != "field" || len(field.Uses) != 1 || field.Source.Line != 6 || !strings.HasSuffix(field.Source.File, "dto/dto.go") {
		t.Fatalf("field source: %+v", field)
	}
	use := field.Uses[0]
	if use.Handler != "example.com/explain.Create" || use.Status != "201" || use.Source.Rule != "test.return" || use.Direction != compiler.Output {
		t.Fatalf("wire use: %+v", use)
	}
	if len(use.Origins) != 1 {
		t.Fatalf("unrelated origins: %+v", use.Origins)
	}
	origin := use.Origins[0]
	if origin.WireName != "Name" || !origin.Required || origin.Schema.MinLength.Value != 3 || len(origin.Declarations) != 1 || origin.Declarations[0].Implementation != "not-proven" || string(origin.Declarations[0].Values["minLength"]) != "3" {
		t.Fatalf("field evidence: %+v", origin)
	}
	generic, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/explain/dto.Page.Value"})
	if err != nil {
		t.Fatal(err)
	}
	if generic.Uses[0].Origins[0].GoType != "example.com/explain.User" || generic.Source.Line != 11 {
		t.Fatalf("generic origin: %+v", generic)
	}
	for _, status := range []string{"201", "400"} {
		response, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/explain.Create", Response: status})
		if err != nil {
			t.Fatal(err)
		}
		if response.Kind != "response" || len(response.Responses) != 1 || response.Responses[0].Response.Value == nil {
			t.Fatalf("response %s: %+v", status, response)
		}
		if status == "400" && (len(response.Declarations) != 1 || response.Declarations[0].Implementation != "not-proven" || len(response.Uses) != 0) {
			t.Fatalf("declaration invented evidence: %+v", response)
		}
	}
	handler, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/explain.Create"})
	if err != nil || handler.Operation == nil || handler.Operation.Symbol != "example.com/explain.Create" {
		t.Fatalf("handler: %+v %v", handler, err)
	}
	for _, query := range []compiler.ExplainQuery{{}, {Symbol: "missing.Field"}, {Symbol: "example.com/explain.Create", Response: "999"}, {Symbol: "example.com/explain/dto.Identity.Name", Response: "201"}} {
		if _, err := result.Explain(query); err == nil {
			t.Fatalf("invalid query accepted: %+v", query)
		}
	}
	for _, name := range []string{"example.com/explain.User.Hidden", "example.com/explain.Unused.Value"} {
		unused, err := result.Explain(compiler.ExplainQuery{Symbol: name})
		if err != nil || len(unused.Uses) != 0 || len(unused.Guidance) == 0 {
			t.Fatalf("omitted field: %+v %v", unused, err)
		}
	}
}

// Keep explanation capture opt-in, deterministic, bounded, and detached from callers.
func TestExplainIsolationAndBudget(t *testing.T) {
	_, options := explainFixture(t)
	calls := 0
	options.Mappers = []compiler.TypeMapper{func(r compiler.ProjectionRequest) (*spec.Schema, bool, error) { calls++; return nil, false, nil }}
	options.Configuration = map[string]json.RawMessage{"mapper": json.RawMessage(`"count-v1"`)}
	plainOptions := options
	plainOptions.Explain = false
	plain, err := compiler.Compile(context.Background(), plainOptions)
	if err != nil {
		t.Fatal(err)
	}
	expectedCalls := calls
	calls = 0
	if _, err := plain.Explain(compiler.ExplainQuery{Symbol: "example.com/explain.Create"}); err == nil {
		t.Fatal("capture silently enabled")
	}
	result, err := compiler.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if calls != expectedCalls {
		t.Fatalf("extra projection: %d != %d", calls, expectedCalls)
	}
	plainJSON, _ := json.Marshal(plain.Bundle.Snapshot())
	capturedJSON, _ := json.Marshal(result.Bundle.Snapshot())
	if string(plainJSON) != string(capturedJSON) {
		t.Fatal("explain changed runtime Bundle")
	}
	q := compiler.ExplainQuery{Symbol: "example.com/explain/dto.Identity.Name"}
	first, err := result.Explain(q)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(first)
	first.Uses[0].Origins[0].Schema.Description = "changed"
	first.Uses[0].Origins[0].Declarations[0].Values["minLength"][0] = '9'
	again, err := result.Explain(q)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(again)
	if string(before) != string(after) || calls != expectedCalls {
		t.Fatal("query mutated capture or invoked mapper")
	}
	options.MaxExplainBytes = 64
	if _, err := compiler.Compile(context.Background(), options); err == nil || !strings.Contains(err.Error(), "openapi.explain.budget") {
		t.Fatalf("budget: %v", err)
	}
	options.MaxExplainBytes = -1
	if _, err := compiler.Compile(context.Background(), options); err == nil {
		t.Fatal("negative budget accepted")
	}
}

// Rename wire fields through the public codec without changing their Go source identity.
type explainCodec struct{}

// Identify the explicit codec profile.
func (explainCodec) Name() string { return "test-renamed-v1" }

// Select exported fields and expose a different wire name.
func (explainCodec) Fields(st *types.Struct) ([]compiler.WireField, error) {
	var fields []compiler.WireField
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Exported() {
			fields = append(fields, compiler.WireField{Name: "wire_" + f.Name(), Field: f, Optional: true})
		}
	}
	return fields, nil
}

// Preserve actual codec names, origin positions, and primitive mapping rules.
func TestExplainProjectionCodec(t *testing.T) {
	dir, _ := explainFixture(t)
	project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: dir, Patterns: []string{"./dto"}, Env: []string{"GOWORK=off"}})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := project.Type("Identity")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := project.Schema(compiler.ProjectionRequest{Type: typ, Direction: compiler.Output, Codec: explainCodec{}, Explain: true})
	if err != nil {
		t.Fatal(err)
	}
	var field *compiler.SchemaOrigin
	for i := range projection.Origins {
		if projection.Origins[i].Kind == "field" {
			field = &projection.Origins[i]
		}
	}
	if field == nil || field.Source.Symbol != "example.com/explain/dto.Identity.Name" || field.WireName != "wire_Name" || field.Required {
		t.Fatalf("codec origin: %+v", field)
	}
	found := false
	for _, rule := range projection.Rules {
		found = found || rule.Rule == "test-renamed-v1"
	}
	if !found {
		t.Fatalf("codec rule missing: %+v", projection.Rules)
	}
	if len(projection.Audit) == 0 {
		t.Fatal("enforcement caveat missing")
	}
}

// Keep public diagnostic types available to external explanation consumers.
var _ openapi.Source

// Keep reporting options out of custom projection semantics and preserve the captured Bundle snapshot.
func TestExplainDoesNotChangeMapperContext(t *testing.T) {
	_, options := explainFixture(t)
	options.Configuration = map[string]json.RawMessage{"mapper": json.RawMessage(`"string-v1"`)}
	options.Mappers = []compiler.TypeMapper{func(r compiler.ProjectionRequest) (*spec.Schema, bool, error) {
		if r.Explain {
			t.Error("reporting flag leaked into mapper inputs")
		}
		if r.Type == types.Typ[types.String] {
			return spec.Typed("string"), true, nil
		}
		return nil, false, nil
	}}
	result, err := compiler.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	result.Bundle = openapi.Bundle{}
	explanation, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/explain.Create", Response: "201"})
	if err != nil || len(explanation.Responses) != 1 {
		t.Fatalf("capture depended on replaceable public Bundle: %+v %v", explanation, err)
	}
	found := false
	for _, use := range explanation.Uses {
		for _, rule := range use.Rules {
			found = found || rule.Rule == "openapi.TypeMapper[0]" && rule.Kind == "declared"
		}
	}
	if !found {
		t.Fatal("selected mapper provenance missing")
	}
}

// Explain conditional bodyless commits and unresolved helper effects without inventing payload fields.
func TestExplainConditionalBodylessAndUnknown(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"go.mod": "module example.com/conditions\n\ngo 1.27.1\n", "app.go": `package app
type Channel struct{}
func (*Channel) Choose(){}
func H(c *Channel){c.Choose()}
`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	front := compiler.Frontend{Name: "conditions-v1", Match: func(f compiler.Function) bool { return f.Object.Name() == "H" }, CallOutcomes: func(c compiler.CallContext) ([]compiler.CallOutcome, error) {
		if c.Object == nil || c.Object.Name() != "Choose" {
			return nil, nil
		}
		source := c.Source
		source.Kind = "derived"
		source.Rule = "test.choose"
		return []compiler.CallOutcome{
			{When: openapi.RequestCondition{Methods: []string{"GET"}}, Effects: []compiler.Effect{{Kind: compiler.ResponseCommit, Status: "204", Source: source}}},
			{When: openapi.RequestCondition{ExceptMethods: []string{"GET"}}, Effects: []compiler.Effect{{Kind: compiler.Unresolved, Message: "custom helper contract is unknown", Fix: "Register the helper in the centralized frontend", Source: source}}},
		}, nil
	}}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []compiler.Frontend{front}, Explain: true})
	if err != nil {
		t.Fatal(err)
	}
	response, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/conditions.H", Response: "204"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Responses) != 1 || response.Responses[0].When == nil || response.Responses[0].When.Methods[0] != "GET" {
		t.Fatalf("response conditions: %+v", response)
	}
	if len(response.Uses) != 1 || response.Uses[0].Status != "204" || response.Uses[0].Codec != "" || response.Uses[0].Source.When == nil || response.Uses[0].Source.Rule != "test.choose" {
		t.Fatalf("commit provenance: %+v", response.Uses)
	}
	handler, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/conditions.H"})
	if err != nil {
		t.Fatal(err)
	}
	if len(handler.Diagnostics) == 0 || !strings.Contains(strings.Join(handler.Guidance, "\n"), "centralized") {
		t.Fatalf("unresolved helper: %+v", handler)
	}
}

// Preserve payload origins before wrapping and return the independently wrapped final response.
func TestExplainTransformedAndFrozenSource(t *testing.T) {
	dir, options := explainFixture(t)
	original := options.Frontends[0].Return
	options.Frontends[0].Return = func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		effects, err := original(c)
		effects[0].TransformSchema = func(payload *spec.Schema) (*spec.Schema, error) {
			wrapper := spec.Typed("object")
			wrapper.Properties = map[string]*spec.Schema{"data": payload}
			return wrapper, nil
		}
		return effects, err
	}
	result, err := compiler.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	first, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/explain.Create", Response: "201"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Uses) != 1 || !first.Uses[0].Transformed || first.Uses[0].Schema.Ref == "" || first.Responses[0].Response.Value.Content["application/json"].Value.Schema.Properties["data"] == nil || first.Components == nil {
		t.Fatalf("payload/final distinction: %+v", first)
	}
	query := compiler.ExplainQuery{Symbol: "example.com/explain/dto.Identity.Name"}
	before, _ := result.Explain(query)
	rawBefore, _ := json.Marshal(before)
	if err := os.WriteFile(filepath.Join(dir, "dto/dto.go"), []byte("package changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := result.Explain(query)
	if err != nil {
		t.Fatal(err)
	}
	rawAfter, _ := json.Marshal(after)
	if string(rawBefore) != string(rawAfter) {
		t.Fatal("explain read changed source instead of its compilation snapshot")
	}
}

// Do not mistake anonymous nested fields for their enclosing unprojected named type.
func TestExplainUnprojectedAnonymousIdentity(t *testing.T) {
	dir, options := explainFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "unused.go"), []byte(`package app
type Outer struct { A struct { Outer string }; B struct { Outer int } }
`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if explanation, err := result.Explain(compiler.ExplainQuery{Symbol: "example.com/explain.Outer"}); err == nil {
		t.Fatalf("invented direct field identity: %+v", explanation)
	}
}
