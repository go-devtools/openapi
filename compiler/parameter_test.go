package compiler

import (
	"context"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
)

// 用中立文本协议模拟非 JSON 指针和字节数组，保留核心注释投影。
// Model non-JSON pointers and byte arrays with a neutral text protocol while retaining core annotations.
type textParameterCodec struct{}

// 为组件提供固定、明确的编解码身份。
// Provide an explicit stable codec identity for components.
func (textParameterCodec) Name() string { return "test-text-parameters-v1" }

// 使用真实字段对象，让核心独立复用其注释和约束。
// Return actual field objects so the core can independently reuse their comments and constraints.
func (textParameterCodec) Fields(s *types.Struct) ([]WireField, error) {
	var fields []WireField
	for i := 0; i < s.NumFields(); i++ {
		if s.Field(i).Exported() {
			fields = append(fields, WireField{Name: s.Field(i).Name(), Field: s.Field(i)})
		}
	}
	return fields, nil
}

// 普通文本参数没有 JSON null 或 Base64 字节序列语义。
// Ordinary text parameters have neither JSON null nor Base64 byte-sequence semantics.
func (textParameterCodec) ProjectType(request ProjectionRequest, project func(types.Type) (*spec.Schema, error)) (*spec.Schema, bool, error) {
	switch value := types.Unalias(request.Type).(type) {
	case *types.Pointer:
		schema, err := project(value.Elem())
		return schema, true, err
	case *types.Slice:
		item, err := project(value.Elem())
		schema := spec.Typed("array")
		schema.Items = item
		return schema, true, err
	}
	return nil, false, nil
}

// 非 JSON 编解码器必须控制网络类型，并通过中立效果展开带注释的参数。
// A non-JSON codec must control wire types and expand annotated parameters through neutral effects.
func TestParameterCodecAndObjectEffects(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
// 中立参数。 Neutral parameters.
type Input struct {
 // 显示名称。 Display name.
 // @openapi required minLength=2
 Name string
 // 字节编号。 Byte identifiers.
 IDs []byte
 // 可选数量。 Optional count.
 Count *int
}
// 接受参数并返回文本。 Accept parameters and return text.
func Handle(input Input) string {return input.Name}
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/parameters\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	project, err := Load(context.Background(), LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := project.Type("Input")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := project.Schema(ProjectionRequest{Type: typ, Direction: Input, MediaType: "application/x-www-form-urlencoded", Codec: textParameterCodec{}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := projection.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(raw, "", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.JSON([]byte(`{"Name":"Ada","IDs":[1,2],"Count":4}`)); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{`{"Name":"A"}`, `{"Name":"Ada","IDs":"AQI="}`, `{"Name":"Ada","Count":null}`} {
		if validator.JSON([]byte(bad)) == nil {
			t.Errorf("accepted invalid parameter object: %s", bad)
		}
	}
	frontend := Frontend{Name: "test-parameter-object-v1", Match: func(f Function) bool { return f.Object.Name() == "Handle" }, Entry: func(f Function) []Effect {
		return []Effect{{Kind: EffectKind("parameterObject"), In: "query", MediaType: "application/x-www-form-urlencoded", Payload: Value{Type: f.Signature.Params().At(0).Type()}, Codec: textParameterCodec{}, Source: f.Source}}
	}, Return: func(c ReturnContext) ([]Effect, error) {
		return []Effect{{Kind: ResponseBody, Status: "200", MediaType: "text/plain", WireSchema: spec.Typed("string"), Source: c.Source}}, nil
	}}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/search", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Parameters", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name      string
		good, bad any
	}{{"Name", "Ada", "A"}, {"IDs", []any{1, 2}, "AQI="}, {"Count", 4, nil}} {
		operation := result.Bundle.Index()[0].Operation
		found := false
		for i, p := range operation.Parameters {
			if p.Value.Name == sample.name {
				found = true
				schema, err := contracttest.Compile(doc.JSON(), "/paths/~1search/get/parameters/"+fmt.Sprint(i)+"/schema", contracttest.Options{})
				if err != nil {
					t.Fatal(err)
				}
				if err := schema.Value(sample.good); err != nil {
					t.Fatal(err)
				}
				if schema.Value(sample.bad) == nil {
					t.Errorf("parameter %s lost its constraint", sample.name)
				}
				if p.Value.Required != (sample.name == "Name") {
					t.Errorf("wrong required flag for %s", sample.name)
				}
			}
		}
		if !found {
			t.Errorf("missing parameter %s", sample.name)
		}
	}
	constrained := strings.Replace(source, "type Input struct {", "// @openapi minProperties=2\ntype Input struct {", 1)
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(constrained), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openapi.Build(changed.Bundle, []openapi.Route{{Method: "GET", Path: "/search", OperationKey: changed.Bundle.Index()[0].Key}}, openapi.Config{Title: "Constrained parameters", Version: "1"}); err == nil {
		t.Fatal("whole-object constraint was silently discarded during parameter expansion")
	}

}

// 模拟扩展提供的递归、错误类型和共享 Schema 返回值。
// Model extension callbacks returning recursion, invalid types, and shared schemas.
type boundaryTypeCodec struct {
	textParameterCodec
	mode   string
	shared *spec.Schema
}

// 通过实际公开回调边界触发资源和所有权检查。
// Exercise resource and ownership checks through the actual public callback boundary.
func (c boundaryTypeCodec) ProjectType(request ProjectionRequest, project func(types.Type) (*spec.Schema, error)) (*spec.Schema, bool, error) {
	switch c.mode {
	case "nil-type":
		schema, err := project(nil)
		return schema, true, err
	case "recursive":
		schema, err := project(request.Type)
		return schema, true, err
	case "nil-schema":
		return nil, true, nil
	default:
		return c.shared, true, nil
	}
}

// 错误扩展必须返回诊断，递归有预算，返回的 Schema 不共享可变状态。
// Invalid extensions must return diagnostics, recursion must be bounded, and returned schemas must not share mutable state.
func TestWireTypeCodecBoundaries(t *testing.T) {
	project := &Project{}
	for _, mode := range []string{"nil-type", "recursive", "nil-schema"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := project.Schema(ProjectionRequest{Type: types.Typ[types.String], Direction: Input, MediaType: "text/plain", Codec: boundaryTypeCodec{mode: mode}, MaxTypes: 16}); err == nil {
				t.Fatalf("invalid codec accepted: %s", mode)
			}
		})
	}
	shared := spec.Typed("string")
	result, err := project.Schema(ProjectionRequest{Type: types.Typ[types.String], Direction: Input, MediaType: "text/plain", Codec: boundaryTypeCodec{shared: shared}})
	if err != nil {
		t.Fatal(err)
	}
	result.Root.Type[0] = "number"
	if shared.Type[0] != "string" {
		t.Fatal("projection changed codec-owned schema")
	}
}
