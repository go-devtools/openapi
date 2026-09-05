package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/swaggerui"
)

// Register a return-value frontend through the public SDK from another module and validate actual call samples.

// 从另一个 module 仅通过公开 SDK 注册返回值前端并校验实际调用样本。
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
		t.Fatalf("候选数量：%d", len(index))
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/users", OperationKey: index[0].Key}}, openapi.Config{Title: "独立前端", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if openapi.Check(doc.JSON()).HasErrors() || !strings.Contains(string(doc.JSON()), "创建用户") {
		t.Fatalf("无效文档：%s", doc.JSON())
	}
	request, err := contracttest.Compile(doc.JSON(), "/paths/~1users/post/requestBody/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := contracttest.Compile(doc.JSON(), "/paths/~1users/post/responses/201/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	input := Request{Name: "小明"}
	raw, _ := json.Marshal(input)
	if err = request.JSON(raw); err != nil {
		t.Fatal(err)
	}
	if err = request.JSON([]byte(`{"Name":"A"}`)); err == nil {
		t.Fatal("声明的最短长度没有生效")
	}
	output, err := Create(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(output)
	if err = response.JSON(raw); err != nil {
		t.Fatal(err)
	}
	if err = response.JSON([]byte(`{"ID":"错误类型","Name":"小明"}`)); err == nil {
		t.Fatal("响应整数类型没有生效")
	}
}

// Simulate a non-net/http consumer and verify defensive copying of shared asset bytes and metadata.

// 模拟非 net/http 传输层消费共享资源，验证字节与元数据的防御性复制。
func TestTransportNeutralResources(t *testing.T) {
	ui, err := swaggerui.New(swaggerui.Config{Title: "独立传输", SpecURL: "./openapi.json"})
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
	if !strings.Contains(string(page.Data), "独立传输") || page.Metadata["X-Content-Type-Options"] != "nosniff" {
		t.Fatal("缺少渲染页面或传输元数据")
	}
	config := string(frames["config.js"].Data)
	if !strings.Contains(config, `"supportedSubmitMethods":[]`) || !strings.Contains(config, `"validatorUrl":null`) {
		t.Fatal("默认安全配置丢失")
	}
	page.Data[0] = '!'
	page.Metadata["Content-Type"] = "invalid"
	fresh, err := ui.Resource("index.html")
	if err != nil || fresh.Bytes()[0] != '<' || fresh.Headers()["Content-Type"] == "invalid" {
		t.Fatal("共享资源被外部修改")
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
	if err = validator.JSON([]byte(`{"Name":"小明"}`)); err != nil {
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
