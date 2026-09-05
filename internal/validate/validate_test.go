package validate

import "testing"

// 验证三点二语义错误和外部引用默认拒绝。
func TestSemanticErrors(t *testing.T) {
	for _, doc := range []string{
		`{"openapi":"3.1.0","info":{"title":"a","version":"1"},"paths":{}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"$ref":"file:///etc/passwd"}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{"/x":{"additionalOperations":{"GET":{"responses":{"200":{}}}}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"tags":[{"name":"a","parent":"b"},{"name":"b","parent":"a"}]}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"type":"string","minimum":1}}}}`,
		`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"type":"string","minLength":10,"maxLength":2}}}}`,
	} {
		if len(Check([]byte(doc))) == 0 {
			t.Errorf("错误接受 %s", doc)
		}
	}
}

// 验证合法布尔 Schema、querystring 和共享媒体类型可以表达。
func TestNative32(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{"/x":{"query":{"parameters":[{"in":"querystring","content":{"application/json":{"schema":true}}}],"responses":{"200":{"content":{"application/x-ndjson":{"$ref":"#/components/mediaTypes/Rows"}}}}}}},"components":{"mediaTypes":{"Rows":{"itemSchema":{"type":"object"}}}}}`)
	if issues := Check(raw); len(issues) != 0 {
		t.Fatalf("合法文档被拒绝：%+v", issues)
	}
}
