package consumer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
)

// 加载带有声明的真实源码，不导入仅在注释中提及的 DTO 包。

// Load actual source with declarations without importing comment-only DTO packages.
func declarationProject(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":     "module example.test/declarations\n\ngo 1.27.1\n",
		"api/api.go": source,
		"dto/dto.go": `package dto
// Share a generic envelope without adding an application import.
type Envelope[T any] struct { Data T }
// Describe an externally declared error.
type Problem struct { Code string }
// Deliberately collide with an API-local name.
type Request struct { Other bool }
`,
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// 使用中立的返回约定，确保注释行为不依赖 Gin。

// Use a neutral return convention so annotation behavior cannot depend on Gin.
func declarationFrontend() compiler.Frontend {
	return compiler.Frontend{
		Name:  "declaration-test-v1",
		Match: func(f compiler.Function) bool { return f.Package.Name == "api" },
		Entry: func(f compiler.Function) []compiler.Effect {
			if f.Signature.Params().Len() == 0 {
				return nil
			}
			return []compiler.Effect{{Kind: compiler.RequestBody, MediaType: "application/json", Payload: compiler.Value{Type: f.Signature.Params().At(0).Type()}, Source: f.Source}}
		},
		Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
			if len(c.Values) == 0 {
				return nil, nil
			}
			return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "201", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
		},
	}
}

// 解析局部及完整限定的泛型类型，保留来源，并独立验证真实实例。

// Resolve local and fully qualified generic types, keep provenance, and validate real instances independently.
func TestCommentDeclarations(t *testing.T) {
	source := `package api
// Describe the local request.
type Request struct { Name string }
// Describe the local response.
type Reply struct { ID int64 }
// Declare a contract for an otherwise effect-free entry.
// @openapi request mediaType="application/json" type="Request" required
// @openapi response status=201 mediaType="application/json" type="example.test/declarations/dto.Envelope[Reply]"
// @openapi response status="default" mediaType="application/json" type="example.test/declarations/dto.Problem"
func Declared() {}
// Repeat the observed contract without replacing its facts.
// @openapi request mediaType="application/json" type="Request" required
// @openapi response status=201 mediaType="application/json" type="Reply"
func Consistent(req Request) Reply { return Reply{ID: 42} }
// Keep unused unresolved declarations isolated from selected routes.
// @openapi response status=200 mediaType="application/json" type="missing.test/never.Download"
func Unselected() {}
`
	dir := declarationProject(t, source)
	options := compiler.Options{Load: compiler.LoadOptions{Dir: dir, Patterns: []string{"./..."}, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{declarationFrontend()}}
	result, err := compiler.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{
		{Method: "POST", Path: "/declared", OperationKey: "example.test/declarations/api.Declared"},
		{Method: "POST", Path: "/consistent", OperationKey: "example.test/declarations/api.Consistent"},
	}, openapi.Config{Title: "Declarations", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(doc.JSON(), "/paths/~1declared/post/responses/201/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.JSON([]byte(`{"Data":{"ID":42}}`)); err != nil {
		t.Fatal(err)
	}
	if validator.JSON([]byte(`{"Data":{"ID":"wrong"}}`)) == nil {
		t.Fatal("declared generic argument lost its actual type")
	}
	input, err := contracttest.Compile(doc.JSON(), "/paths/~1declared/post/requestBody/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := input.JSON([]byte(`{"Name":"Ada"}`)); err != nil {
		t.Fatal(err)
	}
	if input.JSON([]byte(`{"Name":42}`)) == nil {
		t.Fatal("local package resolution selected another Request")
	}
	facts := doc.Report().Facts
	found := false
	for _, fact := range facts {
		if fact.Kind == "declared" && fact.Rule == "openapi.comment.response" && fact.File == "api/api.go" && fact.Line == 8 && fact.Symbol == "example.test/declarations/api.Declared" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing exact declaration source: %+v", facts)
	}
	again, err := compiler.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Bundle.JSON()) != string(result.Bundle.JSON()) {
		t.Fatal("declaration generation is not deterministic")
	}
	output := compiler.WriteOptions{Dir: filepath.Join(dir, "generated"), Package: "generated"}
	if err := result.Write(output); err != nil {
		t.Fatal(err)
	}
	if err := result.Check(output); err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(filepath.Join(dir, "api/api.go"))
	if err != nil || string(unchanged) != source {
		t.Fatal("compilation modified business source", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.sum")); !os.IsNotExist(err) {
		t.Fatalf("comment-only type resolution changed module state: %v", err)
	}
	_, err = openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/bad", OperationKey: "example.test/declarations/api.Unselected"}}, openapi.Config{Title: "Bad", Version: "1"})
	if err == nil || !strings.Contains(err.Error(), "openapi.declaration.type") {
		t.Fatalf("unloaded module type was not diagnosed: %v", err)
	}
}

// 拒绝格式错误的契约及与已观察类型冲突的声明，不伪造成功的回退响应。

// Reject malformed contracts and conflicting observed types without fabricating successful fallback responses.
func TestCommentDeclarationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, directive, code string
		unknown               bool
	}{
		{"request type", `request mediaType="application/json" type="Reply"`, "openapi.declaration.conflict", false},
		{"response type", `response status=201 mediaType="application/json" type="Request"`, "openapi.declaration.conflict", false},
		{"wrong status type", `response status=true mediaType="application/json" type="Reply"`, "openapi.declaration.invalid", false},
		{"missing media", `response status=201 type="Reply"`, "openapi.declaration.invalid", false},
		{"unknown key", `response status=201 mediaType="application/json" type="Reply" required`, "openapi.declaration.invalid", false},
		{"null required", `request mediaType="application/json" type="Request" required=null`, "openapi.declaration.invalid", false},
		{"bodyless status", `response status=204 mediaType="application/json" type="Reply"`, "openapi.declaration.invalid", false},
		{"unbound generic", `response status=201 mediaType="application/json" type="example.test/declarations/dto.Envelope"`, "openapi.declaration.type", false},
		{"non type", `response status=201 mediaType="application/json" type="1+2"`, "openapi.declaration.type", false},
		{"unknown effect", `response status="default" mediaType="application/json" type="Reply"`, "opaque effect", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := declarationProject(t, "package api\ntype Request struct { Name string }\ntype Reply struct { ID int64 }\n// Contract.\n// @openapi "+tc.directive+"\nfunc Handler(req Request) Reply { return Reply{ID:42} }\n")
			frontend := declarationFrontend()
			if tc.unknown {
				entry := frontend.Entry
				frontend.Entry = func(f compiler.Function) []compiler.Effect {
					return append(entry(f), compiler.Effect{Kind: compiler.Unresolved, Source: f.Source, Message: "opaque effect", Fix: "Register the actual helper rule"})
				}
			}
			result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Patterns: []string{"./..."}, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{frontend}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/test", OperationKey: "example.test/declarations/api.Handler"}}, openapi.Config{Title: "Test", Version: "1"})
			if err == nil || !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
}

// 解析已加载的限定泛型表达式，拒绝有歧义的短名称。

// Resolve loaded qualified generic expressions while rejecting ambiguous short names.
func TestQualifiedTypeExpressions(t *testing.T) {
	dir := declarationProject(t, "package api\ntype Request struct { Name string }\ntype Reply struct { ID int64 }\n")
	project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: dir, Patterns: []string{"./..."}, Env: []string{"GOWORK=off", "GOPROXY=off"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"example.test/declarations/dto.Envelope[example.test/declarations/api.Reply]", "[]*example.test/declarations/dto.Problem"} {
		if _, err := project.Type(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := project.Type("Request"); err == nil || !strings.Contains(err.Error(), "openapi.type.ambiguous") {
		t.Fatalf("ambiguous local type was silently selected: %v", err)
	}
}

// 普通说明提及指令时仍保留源码坐标，并拒绝相互矛盾的重复存在性声明。

// Preserve source coordinates when prose mentions directives and reject contradictory repeated presence declarations.
func TestDeclarationSourceAndDuplicateConstraints(t *testing.T) {
	source := `package api
// Explain the @openapi syntax in prose without changing directive positions.
// @openapi tags=["Contracts"]
/* @openapi response status=204 */
func Empty() {}
// Declare contradictory body presence in separate directives.
// @openapi request mediaType="application/json" type="any" required=false
// @openapi request mediaType="application/json" type="any" required=true
// @openapi response status=204
func Contradiction() {}
// Explicitly document unconstrained JSON instead of guessing a missing type.
// @openapi response status=200 mediaType="application/json" type="any"
func Open() {}
`
	dir := declarationProject(t, source)
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Patterns: []string{"./..."}, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{declarationFrontend()}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "DELETE", Path: "/empty", OperationKey: "example.test/declarations/api.Empty"}, {Method: "GET", Path: "/open", OperationKey: "example.test/declarations/api.Open"}}, openapi.Config{Title: "Test", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range doc.Report().Facts {
		if fact.Symbol == "example.test/declarations/api.Empty" && fact.Kind == "declared" {
			found = fact.Line == 4 && fact.Column == 4
		}
	}
	if !found {
		t.Fatalf("prose shifted directive coordinates: %+v", doc.Report().Facts)
	}
	check, err := contracttest.Compile(doc.JSON(), "/paths/~1open/get/responses/200/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`null`, `42`, `"text"`, `{"nested":[1,true]}`} {
		if err := check.JSON([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/conflict", OperationKey: "example.test/declarations/api.Contradiction"}}, openapi.Config{Title: "Conflict", Version: "1"})
	if err == nil || !strings.Contains(err.Error(), "openapi.declaration.conflict") {
		t.Fatalf("contradictory declarations were accepted: %v", err)
	}
}

// 即使显式记录了默认响应，也保留不确定状态及选中路径的错误。

// Keep uncertain status and selected-path errors even when a default response is explicitly documented.
func TestDeclarationDoesNotResolveUnknownStatus(t *testing.T) {
	source := `package api
type Reply struct { ID int64 }
// @openapi response status="default" mediaType="application/json" type="Reply"
func Handler() Reply { return Reply{ID:1} }
`
	dir := declarationProject(t, source)
	frontend := declarationFrontend()
	frontend.Return = func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		return []compiler.Effect{{Kind: compiler.ResponseBody, MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
	}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Patterns: []string{"./..."}, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/status", OperationKey: "example.test/declarations/api.Handler"}}, openapi.Config{Title: "Status", Version: "1"})
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("default declaration swallowed unknown status: %v", err)
	}
}

// 在独立作用域中求值纯 Go 类型表达式，不改写字面量数据或已加载的对象身份。

// Evaluate pure Go type expressions in a detached scope without rewriting literal data or mutating loaded identities.
func TestTypeExpressionScope(t *testing.T) {
	dir := declarationProject(t, "package api\ntype Request struct { Name string }\ntype Reply struct { ID int64 }\n")
	project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: dir, Patterns: []string{"./..."}, Env: []string{"GOWORK=off", "GOPROXY=off"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := "example.test/declarations/api"
	local, err := project.TypeIn(owner, "Request")
	if err != nil {
		t.Fatal(err)
	}
	qualified, err := project.Type("example.test/declarations/api.Request")
	if err != nil || local != qualified {
		t.Fatal("type resolution changed original type identity", err)
	}
	typ, err := project.TypeIn(owner, `[len("example.test/declarations/dto.Problem")]byte`)
	if err != nil || typ.String() != "[37]byte" {
		t.Fatalf("type-like string literal was rewritten: %v %v", typ, err)
	}
	typ, err = project.TypeIn(owner, "[len(`"+strings.Repeat("\r", 100)+"example.test/declarations/dto.Problem`)]byte")
	if err != nil || typ.String() != "[37]byte" {
		t.Fatalf("raw literal normalization changed type resolution: %v %v", typ, err)
	}
	for _, invalid := range []string{"example.test/missing.Type", "example.test/declarations/dto.Envelope", "Reply | Request", strings.Repeat("*", 8193) + "Reply"} {
		if _, err := project.TypeIn(owner, invalid); err == nil {
			t.Fatalf("accepted invalid or oversized type expression: %.100s", invalid)
		}
	}
	for _, pkg := range project.Packages {
		for _, name := range pkg.Types.Scope().Names() {
			if strings.HasPrefix(name, "_openapi_type_") {
				t.Fatal("resolver modified the loaded package scope")
			}
		}
	}
}

// 在每个选中变体上保留声明分支及源码条件，不隐藏不受支持的变体。

// Keep declared alternatives and source conditions on each selected variant without hiding unsupported variants.
func TestConditionalDeclarations(t *testing.T) {
	source := `package api
type Channel struct{}
func (*Channel) Choose() {}
type Reply struct { ID int64 }
// @openapi response status="default" mediaType="application/json" type="Reply"
func Handler(c *Channel) { c.Choose() }
`
	dir := declarationProject(t, source)
	frontend := compiler.Frontend{Name: "conditional-declaration-v1", Match: func(f compiler.Function) bool { return f.Object.Name() == "Handler" }, CallOutcomes: func(c compiler.CallContext) ([]compiler.CallOutcome, error) {
		if c.Object == nil || c.Object.Name() != "Choose" {
			return nil, nil
		}
		typ := c.Function.Package.Types.Scope().Lookup("Reply").Type()
		return []compiler.CallOutcome{
			{When: openapi.RequestCondition{Methods: []string{"GET"}}, Effects: []compiler.Effect{{Kind: compiler.ResponseBody, Status: "200", MediaType: "application/json", Payload: compiler.Value{Type: typ}, Source: c.Source}}},
			{When: openapi.RequestCondition{ExceptMethods: []string{"GET"}}, Effects: []compiler.Effect{{Kind: compiler.Unresolved, Message: "unsupported conditional effect", Source: c.Source}}},
		}, nil
	}}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Patterns: []string{"./..."}, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/condition", OperationKey: "example.test/declarations/api.Handler"}}, openapi.Config{Title: "Conditions", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(document.JSON()), `"default"`) {
		t.Fatal("conditional declaration was lost")
	}
	found := false
	for _, fact := range document.Report().Facts {
		if fact.Kind == "declared" && fact.When != nil && len(fact.When.Methods) == 1 && fact.When.Methods[0] == "GET" {
			found = true
		}
	}
	if !found {
		t.Fatal("declared provenance lost its request condition")
	}
	_, err = openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/condition", OperationKey: "example.test/declarations/api.Handler"}}, openapi.Config{Title: "Conditions", Version: "1"})
	if err == nil || !strings.Contains(err.Error(), "unsupported conditional effect") {
		t.Fatalf("unresolved selected variant was hidden: %v", err)
	}
}
