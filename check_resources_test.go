package openapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openapi-golang/openapi"
)

// 查找公开报告中的稳定错误编码，不绑定可调整的提示文本。
// Find stable error codes in public reports without depending on editable message text.
func hasCode(report openapi.Report, code string) bool {
	for _, d := range report.Diagnostics {
		if d.Code == "openapi.spec."+code {
			return true
		}
	}
	return false
}

// 参照 OAS 三点二的多文档示例，验证检索地址、$self 和 $id 各自改变基准。
// Verify retrieval addresses, $self, and $id change their respective bases using the OAS 3.2 multi-document example.
func TestCheckExplicitOfflineResources(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","$self":"/api/openapi","info":{"title":"main","version":"1"},"paths":{"/foo":{"post":{"requestBody":{"$ref":"shared/foo#/components/requestBodies/Foo"},"responses":{"200":{"description":"OK"}}}}}}`)
	options := openapi.CheckOptions{BaseURI: "https://example.test/staging/openapi.json", Resources: map[string][]byte{
		"https://example.test/staging/foo.json":        []byte(`{"openapi":"3.2.0","$self":"/api/shared/foo","info":{"title":"shared","version":"1"},"components":{"requestBodies":{"Foo":{"content":{"application/json":{"schema":{"$ref":"../schemas/foo"}}}}}}}`),
		"https://example.test/staging/foo-schema.json": []byte(`{"$id":"/api/schemas/foo","type":"object","properties":{"bar":{"$ref":"bar"}}}`),
		"https://example.test/staging/bar-schema.json": []byte(`{"$id":"/api/schemas/bar","type":"boolean"}`),
	}}
	if report := openapi.CheckWithOptions(raw, options); report.HasErrors() {
		t.Fatal(report)
	}
	if report := openapi.Check(raw); !hasCode(report, "external.denied") {
		t.Fatal("未提供资源时应拒绝", report)
	}
	options.Resources["https://example.test/staging/bar-schema.json"] = []byte(`{"$id":"/other/bar","type":"boolean"}`)
	if report := openapi.CheckWithOptions(raw, options); !hasCode(report, "external.denied") {
		t.Fatal("错误基准仍被接受", report)
	}
}

// 允许列表提供真实内容；列出地址并不授权网络请求。
// Allowlisted resources supply actual content; listing an address never authorizes a network request.
func TestCheckNeverFetchesResources(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write([]byte(`true`)) }))
	defer server.Close()
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"schemas":{"A":{"$ref":"` + server.URL + `/schema"}}}}`)
	if report := openapi.Check(raw); !hasCode(report, "external.denied") {
		t.Fatal(report)
	}
	options := openapi.CheckOptions{Resources: map[string][]byte{server.URL + "/schema": []byte(`true`)}}
	if report := openapi.CheckWithOptions(raw, options); report.HasErrors() {
		t.Fatal(report)
	}
	if hits.Load() != 0 {
		t.Fatalf("检查器访问了网络 %d 次", hits.Load())
	}
}

// 原始外部示例只检查提供性，不把内容中的引用当作加载指令。
// Check only whether raw external examples are supplied; their references are not loading instructions.
func TestCheckExplicitExampleResources(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","$self":"https://example.test/api/openapi.json","info":{"title":"a","version":"1"},"components":{"examples":{"Message":{"externalValue":"../samples/message.txt"}}}}`)
	options := openapi.CheckOptions{ExampleResources: map[string][]byte{"https://example.test/samples/message.txt": []byte("event: update\ndata: {\"$ref\":\"file:///secret\"}\n\n")}}
	if report := openapi.CheckWithOptions(raw, options); report.HasErrors() {
		t.Fatal(report)
	}
	if report := openapi.Check(raw); !hasCode(report, "external.denied") {
		t.Fatal(report)
	}
}

// 总预算覆盖所有资源，循环引用只检查边而不无限展开。
// Apply aggregate budgets to all resources and check cyclic edges without expanding them indefinitely.
func TestCheckResourceBudgets(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"schemas":{"A":{"$ref":"https://example.test/a"}}}}`)
	a, b := []byte(`{"$ref":"b"}`), []byte(`{"$ref":"a"}`)
	resources := map[string][]byte{"https://example.test/a": a, "https://example.test/b": b}
	total := len(raw) + len(a) + len(b)
	good := openapi.CheckOptions{Resources: resources, MaxBytes: total, MaxResources: 3, MaxReferences: 3}
	if report := openapi.CheckWithOptions(raw, good); report.HasErrors() {
		t.Fatal(report)
	}
	for _, bad := range []openapi.CheckOptions{
		{Resources: resources, MaxBytes: total - 1}, {Resources: resources, MaxResources: 2}, {Resources: resources, MaxReferences: 2},
	} {
		if report := openapi.CheckWithOptions(raw, bad); !hasCode(report, "budget") {
			t.Fatalf("预算没有生效：%+v %+v", bad, report)
		}
	}
	for _, bad := range []openapi.CheckOptions{{MaxBytes: -1}, {MaxResources: -1}, {MaxReferences: -1}, {BaseURI: "relative.json"}, {BaseURI: "https://example.test/doc#part"}} {
		if report := openapi.CheckWithOptions(raw, bad); !hasCode(report, "options") {
			t.Fatalf("错误配置被忽略：%+v %+v", bad, report)
		}
	}
	many := map[string][]byte{}
	for i := 0; i < 64; i++ {
		many["https://example.test/"+strings.Repeat("a", i+1)] = []byte(`true`)
	}
	if report := openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: many}); !hasCode(report, "budget") {
		t.Fatal("默认资源数未限制", report)
	}
}

// 不完整 JSON、重复键、身份冲突与资源种类错误必须报告资源位置。
// Report resource locations for incomplete JSON, duplicate keys, identity conflicts, and incorrect resource kinds.
func TestCheckInvalidPreloadedResources(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"schemas":{"A":{"$ref":"https://example.test/a"}}}}`)
	for _, tc := range []struct{ key, body, code string }{
		{"https://example.test/a", `{"type":"string","type":"number"}`, "json"},
		{"https://example.test/a", `true false`, "json"},
		{"https://example.test/a", `[]`, "resource.type"},
		{"relative", `true`, "resource.uri"},
		{"https://example.test/a#part", `true`, "resource.uri"},
	} {
		report := openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{tc.key: []byte(tc.body)}})
		if !hasCode(report, tc.code) {
			t.Fatalf("未报告 %s：%+v", tc.code, report)
		}
	}
	report := openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{"https://example.test/a": []byte(`{"$id":"https://example.test/b"}`), "https://example.test/b": []byte(`false`)}})
	if !hasCode(report, "resource.duplicate") {
		t.Fatal("重复资源未拒绝", report)
	}
	snapshot, _ := json.Marshal(report)
	for i := 0; i < 10; i++ {
		again, _ := json.Marshal(openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{"https://example.test/a": []byte(`{"$id":"https://example.test/b"}`), "https://example.test/b": []byte(`false`)}}))
		if string(again) != string(snapshot) {
			t.Fatal("诊断顺序不稳定")
		}
	}
}

// 内嵌 $id 同样占用资源预算，累计 JSON 节点不能通过拆分文档绕过限制。
// Count embedded $id resources and aggregate JSON nodes so splitting documents cannot bypass limits.
func TestCheckEmbeddedAndAggregateBudgets(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"schemas":{"A":{"$id":"a"},"B":{"$id":"b"}}}}`)
	if report := openapi.CheckWithOptions(raw, openapi.CheckOptions{MaxResources: 2}); !hasCode(report, "budget") {
		t.Fatal("内嵌资源未纳入预算", report)
	}
	if report := openapi.CheckWithOptions(raw, openapi.CheckOptions{MaxResources: 3}); report.HasErrors() {
		t.Fatal(report)
	}
	many := []byte(`{"examples":[` + strings.Repeat(`0,`, 100000) + `0]}`)
	report := openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{"https://example.test/a": many, "https://example.test/b": many}})
	if !hasCode(report, "budget") {
		t.Fatal("跨文档 JSON 节点总预算未报告", report)
	}
}
