package compiler_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
)

// 用无框架源码编译有序响应效果，测试只访问公开 SDK。
// Compile ordered response effects from framework-free source through the public SDK only.
func compileItemFixture(t *testing.T, body string, call func(core.CallContext) ([]core.Effect, error)) *core.Result {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.test/items\n\ngo 1.27.1\n",
		"app.go": "package items\ntype Item struct { Name string }\ntype Other struct { Count int }\nfunc Emit(any){}\nfunc Whole(any){}\nfunc Different(any){}\nfunc Status(int){}\nfunc Handle(flag bool){" + body + "}\n",
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := core.Compile(context.Background(), core.Options{
		Load:      core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}},
		Frontends: []core.Frontend{{Name: "neutral-stream-items-v1", Match: func(f core.Function) bool { return f.Object.Name() == "Handle" }, Call: call}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// 使用明确的逐项网络表示，保留普通完整正文与流式正文的区别。
// Supply explicit item wire schemas while distinguishing streams from complete response bodies.
func itemWireEffects(c core.CallContext) ([]core.Effect, error) {
	if c.Object == nil || c.Object.Pkg() == nil || c.Object.Pkg().Path() != "example.test/items" {
		return nil, nil
	}
	if c.Object.Name() == "Status" {
		return []core.Effect{{Kind: core.ResponseStatus, Status: c.Arguments[0].Constant.ExactString(), Source: c.Source}}, nil
	}
	schema := spec.Typed("object")
	schema.Properties = map[string]*spec.Schema{"Name": spec.Typed("string")}
	schema.Required = spec.Set([]string{"Name"})
	if strings.HasSuffix(c.Arguments[0].Type.String(), ".Other") {
		schema.Properties = map[string]*spec.Schema{"Count": spec.Typed("integer")}
		schema.Required = spec.Set([]string{"Count"})
	}
	effect := core.Effect{Kind: core.EffectKind("responseItem"), Status: "-1", MediaType: "application/x-ndjson", WireSchema: schema, Source: c.Source}
	if c.Object.Name() == "Whole" {
		effect.Kind = core.ResponseBody
	}
	if c.Object.Name() == "Different" {
		effect.MediaType = "text/event-stream"
	}
	return []core.Effect{effect}, nil
}

// 连续条目共享 itemSchema；独立验证器接受重叠备选并拒绝错误字段。
// Consecutive items share itemSchema; an independent validator accepts overlapping alternatives and rejects invalid fields.
func TestResponseItems(t *testing.T) {
	result := compileItemFixture(t, `Status(202); Emit(Item{"first"}); Emit(Other{2}); Emit(Item{"last"})`, itemWireEffects)
	route := openapi.Route{Method: "GET", Path: "/items", OperationKey: result.Bundle.Index()[0].Key}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{route}, openapi.Config{Title: "Items", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var model spec.OpenAPI
	if err := json.Unmarshal(doc.JSON(), &model); err != nil {
		t.Fatal(err)
	}
	media := model.Paths["/items"].Get.Responses["202"].Value.Content["application/x-ndjson"].Value
	if media == nil || media.ItemSchema == nil || media.Schema != nil {
		t.Fatalf("incorrect framing: %#v", media)
	}
	validator, err := contracttest.Compile(doc.JSON(), "/paths/~1items/get/responses/202/content/application~1x-ndjson/itemSchema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.NDJSON(strings.NewReader("{\"Name\":\"first\"}\n{\"Count\":2}\n{\"Name\":\"both\",\"Count\":3}\n"), contracttest.Limits{}); err != nil {
		t.Fatal(err)
	}
	if validator.NDJSON(strings.NewReader("{\"Name\":false,\"Count\":\"wrong\"}\n"), contracttest.Limits{}) == nil {
		t.Fatal("invalid item passed")
	}
	route.Method = "HEAD"
	head, err := openapi.Build(result.Bundle, []openapi.Route{route}, openapi.Config{Title: "Items", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(head.JSON(), &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Paths["/items"].Head.Responses["202"].Value.Content) != 0 {
		t.Fatal("HEAD retained stream content")
	}
}

// 不兼容的分帧方式必须诊断，不能将实际连续输出伪装成互斥备选。
// Diagnose incompatible framing instead of disguising sequential output as response alternatives.
func TestResponseItemFramingConflicts(t *testing.T) {
	for _, body := range []string{
		`Emit(Item{}); Whole(Item{})`,
		`Whole(Item{}); Emit(Item{})`,
		`Emit(Item{}); Different(Item{})`,
		`if flag { Emit(Item{}) } else { Whole(Item{}) }`,
		`if flag { Whole(Item{}) } else { Emit(Item{}) }`,
	} {
		t.Run(body, func(t *testing.T) {
			result := compileItemFixture(t, body, itemWireEffects)
			_, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/items", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Items", Version: "1"})
			if err == nil {
				t.Fatal("incompatible stream framing was accepted")
			}
		})
	}
}

// 无正文状态在投影前消除条目，不会因未发送的未知 payload 误报。
// Bodyless statuses discard items before projection, avoiding errors for payloads never sent.
func TestBodylessResponseItems(t *testing.T) {
	for _, status := range []string{"204", "304"} {
		result := compileItemFixture(t, "Status("+status+"); Emit(Item{})", func(c core.CallContext) ([]core.Effect, error) {
			if c.Object.Name() == "Status" {
				return []core.Effect{{Kind: core.ResponseStatus, Status: c.Arguments[0].Constant.ExactString(), Source: c.Source}}, nil
			}
			return []core.Effect{{Kind: core.EffectKind("responseItem"), Status: "-1", MediaType: "application/x-ndjson", Payload: core.Value{Unknown: true}, Source: c.Source}}, nil
		})
		doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/items", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Items", Version: "1"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(doc.JSON()), "itemSchema") {
			t.Fatal("bodyless response gained content")
		}
	}
}

// 条件合并必须保留逐项约束；完整正文与逐项分帧不能同时消失。
// Conditional merging must retain item constraints rather than erasing both complete and item framing.
func TestConditionalItemFraming(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			first := spec.MediaType{ItemSchema: spec.Typed("string")}
			second := spec.MediaType{ItemSchema: spec.Typed("integer")}
			if mixed {
				second = spec.MediaType{Schema: spec.Typed("string")}
			}
			if reverse {
				first, second = second, first
			}
			operation := func(media spec.MediaType) spec.Operation {
				return spec.Operation{Responses: map[string]spec.RefOr[spec.Response]{"200": spec.Inline(spec.Response{Content: map[string]spec.RefOr[spec.MediaType]{"application/x-ndjson": spec.Inline(media)}})}}
			}
			bundle, err := openapi.NewBundle(openapi.BundleData{FormatVersion: 1, SpecVersion: "3.2.0", Capabilities: []string{openapi.RequestConditionsCapability}, Templates: []openapi.Template{{Key: "items", Variants: []openapi.OperationVariant{
				{Operation: operation(first)}, {When: openapi.RequestCondition{Methods: []string{"GET"}}, Operation: operation(second)},
			}}}})
			if err != nil {
				t.Fatal(err)
			}
			doc, err := openapi.Build(bundle, []openapi.Route{{Method: "GET", Path: "/items", OperationKey: "items"}}, openapi.Config{Title: "Items", Version: "1"})
			if mixed {
				if err == nil {
					t.Fatal("conditional framing constraints were silently erased")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			validator, err := contracttest.Compile(doc.JSON(), "/paths/~1items/get/responses/200/content/application~1x-ndjson/itemSchema", contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err := validator.NDJSON(strings.NewReader("\"one\"\n2\n"), contracttest.Limits{}); err != nil {
				t.Fatal(err)
			}
			if validator.Value(false) == nil {
				t.Fatal("conditional item constraints were lost")
			}
		}
	}
}
