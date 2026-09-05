package swaggerui

import (
	"strings"
	"testing"
)

// 非 net/http 消费者直接读取同一份 UI 资源与安全元数据。
func TestNeutralResourcesAndSafeDefaults(t *testing.T) {
	ui, err := New(Config{Title: "用户文档"})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := ui.Resource("config.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(resource.Bytes())
	for _, want := range []string{`"validatorUrl":null`, `"persistAuthorization":false`, `"queryConfigEnabled":false`, `"supportedSubmitMethods":[]`} {
		if !strings.Contains(text, want) {
			t.Errorf("安全默认项缺失 %s：%s", want, text)
		}
	}
	js, err := ui.Resource("swagger-ui-bundle.js")
	if err != nil || len(js.Bytes()) < 100000 {
		t.Fatalf("缺少真实 Swagger UI：%v", err)
	}
	original := js.Bytes()
	changed := js.Bytes()
	changed[0] ^= 0xff
	if string(js.Bytes()) != string(original) {
		t.Fatal("资源可被外部篡改")
	}
	headers := js.Headers()
	headers["Content-Type"] = "bad"
	if js.Headers()["Content-Type"] == "bad" {
		t.Fatal("元数据共享 map")
	}
	if js.Headers()["ETag"] == "" || js.Headers()["X-Content-Type-Options"] != "nosniff" {
		t.Fatal("缺少资源安全元数据")
	}
}

// 验证默认无法通过 URL、目录穿越或脚本注入切换远端规范。
func TestRejectsRemoteAndTraversal(t *testing.T) {
	for _, url := range []string{"https://evil.test/spec", "//evil.test/spec", "../spec.json", "/spec.json?url=https://evil.test"} {
		if _, err := New(Config{SpecURL: url}); err == nil {
			t.Errorf("错误接受 %q", url)
		}
	}
	ui, err := New(Config{Title: "</title><script>alert(1)</script>"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := ui.Resource("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page.Bytes()), "<script>alert") {
		t.Fatal("标题未转义")
	}
	for _, path := range []string{"../LICENSE", "%2e%2e/LICENSE", "/swagger-ui.css", "a/../swagger-ui.css"} {
		if _, err := ui.Resource(path); err == nil {
			t.Errorf("错误接受资源路径 %q", path)
		}
	}
}

// 只有明确配置才允许指定 HTTP 方法进入提交 UI。
func TestExplicitSubmitMethods(t *testing.T) {
	ui, err := New(Config{SubmitMethods: []string{"get", "post"}})
	if err != nil {
		t.Fatal(err)
	}
	js, err := ui.Resource("config.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js.Bytes()), `"supportedSubmitMethods":["get","post"]`) {
		t.Fatal("显式提交配置丢失")
	}
}

// 分组过滤、展开方式与排序是显式配置，不能注入任意可执行排序函数。
func TestGroupingConfiguration(t *testing.T) {
	ui, err := New(Config{Filter: true, DocExpansion: "none", TagsSorter: "alpha", OperationsSorter: "method"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := ui.Resource("config.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"filter":true`, `"docExpansion":"none"`, `"tagsSorter":"alpha"`, `"operationsSorter":"method"`} {
		if !strings.Contains(string(r.Bytes()), want) {
			t.Errorf("缺少分组设置 %s", want)
		}
	}
	for _, cfg := range []Config{{DocExpansion: "invalid"}, {TagsSorter: "function(){}"}, {OperationsSorter: "invalid"}} {
		if _, err := New(cfg); err == nil {
			t.Fatal("接受了未知展示配置")
		}
	}
}

// 整体分类使用固定的本地规范列表和上游选择器，禁止远端地址或重复标签。
func TestDocumentDefinitions(t *testing.T) {
	ui, err := New(Config{Definitions: []Definition{{Name: "用户", URL: "./groups/users.json"}, {Name: "管理", URL: "./groups/admin.json"}}, PrimaryDefinition: "用户"})
	if err != nil {
		t.Fatal(err)
	}
	js, _ := ui.Resource("config.js")
	page, _ := ui.Resource("index.html")
	for _, want := range []string{`"urls":[`, `"urls.primaryName":"用户"`, `"layout":"StandaloneLayout"`} {
		if !strings.Contains(string(js.Bytes()), want) {
			t.Fatalf("分类选择器缺少 %s", want)
		}
	}
	if !strings.Contains(string(page.Bytes()), "swagger-ui-standalone-preset.js") {
		t.Fatal("缺少固定离线的选择器资源")
	}
	for _, cfg := range []Config{
		{Definitions: []Definition{{Name: "远端", URL: "https://example.test/spec.json"}}},
		{Definitions: []Definition{{Name: "重复", URL: "./a.json"}, {Name: "重复", URL: "./b.json"}}},
		{Definitions: []Definition{{Name: "用户", URL: "./a.json"}}, PrimaryDefinition: "不存在"},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatal("错误接受无效文档分类")
		}
	}
}
