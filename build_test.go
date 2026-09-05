package openapi

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/openapi-golang/openapi/spec"
)

// 构造一个中立模板，测试不依赖任何 HTTP 框架。
func testBundle(t *testing.T) Bundle {
	t.Helper()
	b, err := NewBundle(BundleData{FormatVersion: 1, SpecVersion: "3.2.0", Templates: []Template{{Key: "user", Symbol: "example.com/app.User", Operation: spec.Operation{Summary: "读取用户", Responses: map[string]spec.RefOr[spec.Response]{"200": spec.Inline(spec.Response{Description: "成功", Content: map[string]spec.RefOr[spec.MediaType]{"application/json": spec.Inline(spec.MediaType{Schema: &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/User"}}})}})}}}}, Components: spec.Components{Schemas: map[string]*spec.Schema{"User": spec.Typed("object"), "Unused": spec.Typed("string")}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// 验证 Bundle 与最终文档通过防御性复制隔离多次构建。
func TestBuildImmutableAndStable(t *testing.T) {
	b := testBundle(t)
	routes := []Route{{Method: "GET", Path: "/users/{id}", OperationKey: "user"}}
	doc, err := Build(b, routes, Config{Title: "用户服务", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	original := string(doc.JSON())
	data := doc.JSON()
	data[0] = '!'
	if string(doc.JSON()) != original {
		t.Fatal("调用者修改了共享 JSON")
	}
	snapshot := b.Snapshot()
	snapshot.Templates[0].Operation.Summary = "篡改"
	if b.Snapshot().Templates[0].Operation.Summary != "读取用户" {
		t.Fatal("快照共享可变状态")
	}
	if strings.Contains(original, "Unused") {
		t.Fatal("未裁剪不可达组件")
	}
	var parsed spec.OpenAPI
	if err = json.Unmarshal(doc.JSON(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Paths["/users/{id}"].Get.Parameters) != 1 {
		t.Fatal("未链接规范路径参数")
	}
	reversed := []Route{routes[0]}
	again, err := Build(b, reversed, Config{Title: "用户服务", Version: "1"})
	if err != nil || string(again.JSON()) != original {
		t.Fatalf("生成结果不稳定：%v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, e := Build(b, routes, Config{Title: "用户服务", Version: "1"})
			if e != nil || string(d.JSON()) != original {
				t.Error("并发链接结果不一致")
			}
		}()
	}
	wg.Wait()
}

// 验证未知模板、重复路由和未解决事实不会伪装成成功文档。
func TestBuildRejectsUnresolvedAndConflicts(t *testing.T) {
	b := testBundle(t)
	for _, routes := range [][]Route{{{Method: "GET", Path: "/a", OperationKey: "missing"}}, {{Method: "GET", Path: "/a", OperationKey: "user"}, {Method: "GET", Path: "/a", OperationKey: "user"}}, {{Method: "GET", Path: "/users/{id", OperationKey: "user"}}} {
		if _, err := Build(b, routes, Config{Title: "服务", Version: "1"}); err == nil {
			t.Fatalf("错误接受路由：%+v", routes)
		}
	}
	data := b.Snapshot()
	data.Templates[0].Diagnostics = []Diagnostic{{Code: "frontend.unknown", Severity: Error, Message: "未知响应", Fix: "注册集中规则"}}
	b, err := NewBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Build(b, []Route{{Method: "GET", Path: "/a", OperationKey: "user"}}, Config{Title: "服务", Version: "1"}); err == nil {
		t.Fatal("未解决事实被隐藏")
	}
	if _, err = Build(b, nil, Config{Title: "服务", Version: "1"}); err != nil {
		t.Fatal("未选中候选污染文档", err)
	}
}

// 验证未来格式和未知必需能力会在读取边界明确失败。
func TestBundleCompatibility(t *testing.T) {
	data := testBundle(t).Snapshot()
	data.FormatVersion = 2
	if _, err := NewBundle(data); err == nil {
		t.Fatal("接受未知格式")
	}
	data.FormatVersion = 1
	data.Capabilities = []string{"future.required"}
	if _, err := NewBundle(data); err == nil {
		t.Fatal("接受未知能力")
	}
}
