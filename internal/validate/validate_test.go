package validate

import (
	"os"
	"testing"
)

// 验证三点二语义错误和外部引用默认拒绝。
func TestSemanticErrors(t *testing.T) {
	for _, doc := range []string{
		`{"openapi":"3.1.0","info":{"title":"a","version":"1"},"paths":{}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"$ref":"file:///etc/passwd"}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{"/x":{"additionalOperations":{"GET":{"responses":{"200":{}}}}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"tags":[{"name":"a","parent":"b"},{"name":"b","parent":"a"}]}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"type":"unknown"}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"type":"string","minLength":-1}}}}`,
	} {
		if len(Check([]byte(doc))) == 0 {
			t.Errorf("错误接受 %s", doc)
		}
	}
}

// 验证合法布尔 Schema、querystring 和共享媒体类型可以表达。
func TestNative32(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{"/x":{"query":{"parameters":[{"name":"query","in":"querystring","content":{"application/json":{"schema":true}}}],"responses":{"200":{"content":{"application/x-ndjson":{"$ref":"#/components/mediaTypes/Rows"}}}}}}},"components":{"mediaTypes":{"Rows":{"itemSchema":{"type":"object"}}}}}`)
	if issues := Check(raw); len(issues) != 0 {
		t.Fatalf("合法文档被拒绝：%+v", issues)
	}
}

// 完整标准样例同时覆盖 Link 参数数据与三点二新增对象的上下文。
func TestFullNative32Fixture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/golden/openapi32-full.json")
	if err != nil {
		t.Fatal(err)
	}
	if issues := Check(raw); len(issues) != 0 {
		t.Fatalf("完整样例被拒绝：%+v", issues)
	}
}

// 参数名称对 querystring 同样必需，Link 的 parameters 则是表达式数据。
func TestQuerystringNameRequired(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"parameters":{"Q":{"in":"querystring","content":{"application/json":{"schema":true}}}}}}`)
	for _, issue := range Check(raw) {
		if issue.Code == "openapi.spec.parameter.name" {
			return
		}
	}
	t.Fatal("无名称的 querystring 参数应被拒绝")
}
