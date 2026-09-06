package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/constant"
	"go/types"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
	"github.com/openapi-golang/openapi/swaggerui"
)

// 从另一个 module 仅通过公开 SDK 注册返回值前端并校验实际调用样本。

// Register a return-value frontend through the public SDK from another module and validate actual call samples.
func TestPublicFrontend(t *testing.T) {
	front := compiler.Frontend{Name: "external-return-v1", Match: func(f compiler.Function) bool {
		return f.Object.Name() == "Create"
	}, Entry: func(f compiler.Function) []compiler.Effect {
		return []compiler.Effect{{Kind: compiler.RequestBody, MediaType: "application/json", Required: true, Payload: compiler.Value{Type: f.Signature.Params().At(0).Type()}, Source: f.Source}}
	}, Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "201", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
	}}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: "."}, Frontends: []compiler.Frontend{front}})
	if err != nil {
		t.Fatal(err)
	}
	index := result.Bundle.Index()
	if len(index) != 1 {
		t.Fatalf("candidate count: %d", len(index))
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/users", OperationKey: index[0].Key}}, openapi.Config{Title: "Independent frontend", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if openapi.Check(doc.JSON()).HasErrors() || !strings.Contains(string(doc.JSON()), "Create a user") {
		t.Fatalf("invalid document: %s", doc.JSON())
	}
	request, err := contracttest.Compile(doc.JSON(), "/paths/~1users/post/requestBody/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := contracttest.Compile(doc.JSON(), "/paths/~1users/post/responses/201/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	input := Request{Name: "\u5c0f\u660e"}
	raw, _ := json.Marshal(input)
	if err = request.JSON(raw); err != nil {
		t.Fatal(err)
	}
	if err = request.JSON([]byte(`{"Name":"A"}`)); err == nil {
		t.Fatal("declared minimum length was not applied")
	}
	output, err := Create(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(output)
	if err = response.JSON(raw); err != nil {
		t.Fatal(err)
	}
	if err = response.JSON([]byte(`{"ID":"wrong type","Name":"Alice"}`)); err == nil {
		t.Fatal("response integer type was not applied")
	}
}

// 模拟非 net/http 传输层消费共享资源，验证字节与元数据的防御性复制。

// Simulate a non-net/http consumer and verify defensive copying of shared asset bytes and metadata.
func TestTransportNeutralResources(t *testing.T) {
	ui, err := swaggerui.New(swaggerui.Config{Title: "Independent transport", SpecURL: "./openapi.json"})
	if err != nil {
		t.Fatal(err)
	}
	type frame struct {
		Data     []byte
		Metadata map[string]string
	}
	frames := map[string]frame{}
	for _, name := range ui.Names() {
		r, err := ui.Resource(name)
		if err != nil {
			t.Fatal(err)
		}
		frames[name] = frame{Data: r.Bytes(), Metadata: r.Headers()}
	}
	page := frames["index.html"]
	if !strings.Contains(string(page.Data), "Independent transport") || page.Metadata["X-Content-Type-Options"] != "nosniff" {
		t.Fatal("rendered page or transport metadata is missing")
	}
	config := string(frames["config.js"].Data)
	if !strings.Contains(config, `"supportedSubmitMethods":[]`) || !strings.Contains(config, `"validatorUrl":null`) {
		t.Fatal("safe defaults were lost")
	}
	page.Data[0] = '!'
	page.Metadata["Content-Type"] = "invalid"
	fresh, err := ui.Resource("index.html")
	if err != nil || fresh.Bytes()[0] != '<' || fresh.Headers()["Content-Type"] == "invalid" {
		t.Fatal("shared resource was modified externally")
	}
}

// 从独立 module 使用公开导出选项，并验证同一投影的并发只读行为。

// Use public export options from an independent module and verify concurrent read-only projection access.
func TestStandaloneSchemaSDK(t *testing.T) {
	project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: "."})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := project.Type("Request")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := project.Schema(compiler.ProjectionRequest{Type: typ, Direction: compiler.Input})
	if err != nil {
		t.Fatal(err)
	}
	resource := []byte(`{"$id":"extra","type":"string"}`)
	options := compiler.StandaloneOptions{
		BaseURI:            "https://consumer.test/schema/request.json",
		Resources:          map[string][]byte{"https://consumer.test/schema/extra": resource},
		MaxNormalizedBytes: 64 << 10,
	}
	before, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := projection.StandaloneWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(raw, "", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON([]byte("{\"Name\":\"\u5c0f\u660e\"}")); err != nil {
		t.Fatal(err)
	}
	if validator.JSON([]byte(`{"Name":"A"}`)) == nil {
		t.Fatal("standalone schema lost the source constraint")
	}
	errors := make(chan error, 8)
	for i := 0; i < cap(errors); i++ {
		go func() {
			got, err := projection.StandaloneWithOptions(options)
			if err == nil && !bytes.Equal(got, raw) {
				err = fmt.Errorf("concurrent export changed its output")
			}
			errors <- err
		}()
	}
	for i := 0; i < cap(errors); i++ {
		if err := <-errors; err != nil {
			t.Error(err)
		}
	}
	after, err := json.Marshal(projection)
	if err != nil || !bytes.Equal(before, after) || string(resource) != `{"$id":"extra","type":"string"}` {
		t.Fatal("export changed caller-owned data")
	}
}

// 从外部 module 提供明确的非 JSON 网络 Schema，验证响应头与共享输入不变性。

// Supply an explicit non-JSON wire schema from an external module and verify headers and shared-input immutability.
func TestExplicitWireResponseSDK(t *testing.T) {
	wire := spec.Typed("string")
	before, _ := json.Marshal(wire)
	frontend := compiler.Frontend{Name: "external-text", Match: func(f compiler.Function) bool { return f.Object.Name() == "TextResult" }, Entry: func(f compiler.Function) []compiler.Effect {
		return []compiler.Effect{{Kind: compiler.ResponseHeader, Name: "X-Source", Payload: compiler.Value{Type: types.Typ[types.String], Constant: constant.MakeString("external")}, Source: f.Source}}
	}, Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "200", MediaType: "text/plain", WireSchema: wire, Source: c.Source}}, nil
	}}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: "."}, Frontends: []compiler.Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	index := result.Bundle.Index()
	if len(index) != 1 {
		t.Fatalf("unexpected candidates: %d", len(index))
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/text", OperationKey: index[0].Key}}, openapi.Config{Title: "External text", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(doc.JSON(), "/paths/~1text/get/responses/200/content/text~1plain/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Value(TextResult(true)); err != nil {
		t.Fatal(err)
	}
	if validator.Value(42) == nil {
		t.Fatal("text response schema accepts a number")
	}
	header, err := contracttest.Compile(doc.JSON(), "/paths/~1text/get/responses/200/headers/X-Source/schema", contracttest.Options{})
	if err != nil || header.Value("external") != nil || header.Value("other") == nil {
		t.Fatal("response header was not preserved")
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("shared frontend schema was mutated")
	}
}

// 外部 codec 控制文本参数的空值和集合形态，仍复用核心字段注释。

// An external codec controls text-parameter nulls and collections while reusing core field annotations.
type parameterCodec struct{}

// 返回固定的公开扩展身份。

// Return a stable public extension identity.
func (parameterCodec) Name() string { return "external-text-v1" }

// 返回标准字段对象，不复制核心注释解析器。

// Return standard field objects without duplicating the core comment parser.
func (parameterCodec) Fields(value *types.Struct) ([]compiler.WireField, error) {
	var fields []compiler.WireField
	for i := 0; i < value.NumFields(); i++ {
		if value.Field(i).Exported() {
			fields = append(fields, compiler.WireField{Name: value.Field(i).Name(), Field: value.Field(i)})
		}
	}
	return fields, nil
}

// 类型回调复用同一次投影的预算、枚举和引用缓存。

// Type callbacks reuse the projection's budget, enums, and reference cache.
func (parameterCodec) ProjectType(request compiler.ProjectionRequest, project func(types.Type) (*spec.Schema, error)) (*spec.Schema, bool, error) {
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

// 从独立模块调用可选 codec 接口并通过中立参数效果构建最终文档。

// Use the optional codec interface from an independent module and build a document through neutral parameter effects.
func TestParameterCodecSDK(t *testing.T) {
	var _ compiler.WireTypeCodec = parameterCodec{}
	frontend := compiler.Frontend{Name: "external-parameter-v1", Match: func(f compiler.Function) bool { return f.Object.Name() == "Find" }, Entry: func(f compiler.Function) []compiler.Effect {
		return []compiler.Effect{{Kind: compiler.ParameterObject, In: "query", Style: "form", Explode: spec.Set(true), MediaType: "application/x-www-form-urlencoded", Payload: compiler.Value{Type: f.Signature.Params().At(0).Type()}, Codec: parameterCodec{}, Source: f.Source}}
	}, Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "200", MediaType: "text/plain", WireSchema: spec.Typed("string"), Source: c.Source}}, nil
	}}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: "."}, Frontends: []compiler.Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/search", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "External parameters", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	op := result.Bundle.Index()[0].Operation
	for i, parameter := range op.Parameters {
		validator, err := contracttest.Compile(doc.JSON(), fmt.Sprintf("/paths/~1search/get/parameters/%d/schema", i), contracttest.Options{})
		if err != nil {
			t.Fatal(err)
		}
		var good, bad any
		switch parameter.Value.Name {
		case "Name":
			good, bad = "Ada", "A"
		case "IDs":
			good, bad = []any{1, 2}, "AQI="
		case "Limit":
			good, bad = 3, nil
		default:
			t.Fatal("unexpected parameter")
		}
		if err := validator.Value(good); err != nil {
			t.Fatal(err)
		}
		if validator.Value(bad) == nil {
			t.Fatalf("lost wire constraint for %s", parameter.Value.Name)
		}
	}
	if len(op.Parameters) != 3 {
		t.Fatal("parameter projection is incomplete")
	}
}
