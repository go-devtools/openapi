package compiler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"testing"
)

// Record unexpected resource access without loading files to hide export defects.
// 记录意外资源访问，不能通过加载外部文件掩盖导出缺陷。
type standaloneLoader struct{ calls int }

// Reject implicit loading so the independent engine uses only the exported document.
// 拒绝隐式加载，独立引擎只能读取导出的单个文档。
func (l *standaloneLoader) Load(uri string) (any, error) {
	l.calls++
	return nil, fmt.Errorf("unexpected load: %s", uri)
}

// Check export bytes with an independent engine, exact numbers, and no external loader.
// 用独立引擎检查导出字节，保留数值精度并禁止外部加载。
func compileStandalone(t *testing.T, raw []byte) *jsonschema.Schema {
	t.Helper()
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	engine := jsonschema.NewCompiler()
	engine.DefaultDraft(jsonschema.Draft2020)
	loader := &standaloneLoader{}
	engine.UseLoader(loader)
	const uri = "https://consumer.test/export.json"
	if err := engine.AddResource(uri, value); err != nil {
		t.Fatal(err)
	}
	schema, err := engine.Compile(uri)
	if err != nil {
		t.Fatal(err)
	}
	if loader.calls != 0 {
		t.Fatalf("implicit loads: %d", loader.calls)
	}
	return schema
}

// Preserve identified component references and dynamic recursion constraints on children.
// 含 $id 的组件引用必须保持资源身份，动态递归仍约束子节点。
func TestStandaloneIdentifiedComponent(t *testing.T) {
	node := spec.Typed("object")
	node.ID = "https://example.test/node"
	node.DynamicAnchor = "node"
	node.Required = spec.Set([]string{"name"})
	node.Properties = map[string]*spec.Schema{
		"name":     spec.Typed("string"),
		"children": {SchemaObject: &spec.SchemaObject{Type: spec.Types{"array"}, Items: &spec.Schema{SchemaObject: &spec.SchemaObject{DynamicRef: "#node"}}}},
	}
	projection := &compiler.Projection{Root: &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/Node"}}, Components: map[string]*spec.Schema{"Node": node}}
	raw, err := projection.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	validator := compileStandalone(t, raw)
	good := map[string]any{"name": "root", "children": []any{map[string]any{"name": "child"}}}
	bad := map[string]any{"name": "root", "children": []any{map[string]any{"name": 12}}}
	if err = validator.Validate(good); err != nil {
		t.Fatal(err)
	}
	if validator.Validate(bad) == nil {
		t.Fatal("recursive child escaped its schema")
	}
	if projection.Root.Ref != "#/components/schemas/Node" || node.ID != "https://example.test/node" {
		t.Fatal("export modified caller data")
	}
}

// Do not reinterpret matching text in a new resource as a root component or local definition.
// 不把新资源内的同形引用误认为根组件或局部定义。
func TestStandaloneDoesNotRewriteAcrossResourceScope(t *testing.T) {
	scoped := spec.Typed("object")
	scoped.ID = "https://example.test/scoped"
	scoped.Defs = map[string]*spec.Schema{"Leaf": spec.Typed("string")}
	scoped.Properties = map[string]*spec.Schema{"value": {SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/Leaf"}}}
	projection := &compiler.Projection{Root: &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: scoped.ID}}, Components: map[string]*spec.Schema{"Scoped": scoped, "Leaf": spec.Typed("integer")}}
	raw, err := projection.Standalone()
	if err == nil || len(raw) > 0 {
		t.Fatalf("nonexistent resource-local reference silently rebound: %s", raw)
	}
}
