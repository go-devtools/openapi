package swaggerui

import (
	"strings"
	"testing"
)

// Verify a non-net/http consumer can read resources and security metadata.
// 非 net/http 消费者直接读取同一份 UI 资源与安全元数据。
func TestNeutralResourcesAndSafeDefaults(t *testing.T) {
	ui, err := New(Config{Title: "User documentation"})
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
			t.Errorf("missing safe default %s: %s", want, text)
		}
	}
	js, err := ui.Resource("swagger-ui-bundle.js")
	if err != nil || len(js.Bytes()) < 100000 {
		t.Fatalf("actual Swagger UI is missing: %v", err)
	}
	original := js.Bytes()
	changed := js.Bytes()
	changed[0] ^= 0xff
	if string(js.Bytes()) != string(original) {
		t.Fatal("resource can be modified externally")
	}
	headers := js.Headers()
	headers["Content-Type"] = "bad"
	if js.Headers()["Content-Type"] == "bad" {
		t.Fatal("metadata shares a map")
	}
	if js.Headers()["ETag"] == "" || js.Headers()["X-Content-Type-Options"] != "nosniff" {
		t.Fatal("resource security metadata is missing")
	}
}

// Reject remote-spec overrides, traversal, and title injection.
// 验证默认无法通过 URL、目录穿越或脚本注入切换远端规范。
func TestRejectsRemoteAndTraversal(t *testing.T) {
	for _, url := range []string{"https://evil.test/spec", "//evil.test/spec", "../spec.json", "/spec.json?url=https://evil.test"} {
		if _, err := New(Config{SpecURL: url}); err == nil {
			t.Errorf("incorrectly accepted %q", url)
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
		t.Fatal("title was not escaped")
	}
	for _, path := range []string{"../LICENSE", "%2e%2e/LICENSE", "/swagger-ui.css", "a/../swagger-ui.css"} {
		if _, err := ui.Resource(path); err == nil {
			t.Errorf("resource path %q was incorrectly accepted", path)
		}
	}
}

// Permit submission methods only through explicit configuration.
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
		t.Fatal("explicit submit configuration was lost")
	}
}

// Accept explicit filtering, expansion, and sorting options without allowing executable sorting functions.
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
			t.Errorf("missing group setting %s", want)
		}
	}
	for _, cfg := range []Config{{DocExpansion: "invalid"}, {TagsSorter: "function(){}"}, {OperationsSorter: "invalid"}} {
		if _, err := New(cfg); err == nil {
			t.Fatal("unknown presentation configuration was accepted")
		}
	}
}

// Use fixed local specification lists with the upstream selector; reject remote URLs and duplicate labels.
// 整体分类使用固定的本地规范列表和上游选择器，禁止远端地址或重复标签。
func TestDocumentDefinitions(t *testing.T) {
	ui, err := New(Config{Definitions: []Definition{{Name: "Users", URL: "./groups/users.json"}, {Name: "Administration", URL: "./groups/admin.json"}}, PrimaryDefinition: "Users"})
	if err != nil {
		t.Fatal(err)
	}
	js, _ := ui.Resource("config.js")
	page, _ := ui.Resource("index.html")
	for _, want := range []string{`"urls":[`, `"urls.primaryName":"Users"`, `"layout":"StandaloneLayout"`} {
		if !strings.Contains(string(js.Bytes()), want) {
			t.Fatalf("definition selector is missing %s", want)
		}
	}
	if !strings.Contains(string(page.Bytes()), "swagger-ui-standalone-preset.js") {
		t.Fatal("fixed offline selector resource is missing")
	}
	for _, cfg := range []Config{
		{Definitions: []Definition{{Name: "Remote", URL: "https://example.test/spec.json"}}},
		{Definitions: []Definition{{Name: "Duplicate", URL: "./a.json"}, {Name: "Duplicate", URL: "./b.json"}}},
		{Definitions: []Definition{{Name: "Users", URL: "./a.json"}}, PrimaryDefinition: "Missing"},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatal("invalid definition was accepted")
		}
	}
}
