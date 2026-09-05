package validate

import (
	"encoding/json"
	"os"
	"testing"
)

// 使用同一组标准关键字样例，与独立官方元 Schema 校验交叉核对。
func TestSchemaKeywordMatrix(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/golden/schema-keywords.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Schema json.RawMessage
		Valid  bool
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			doc := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":` + string(tc.Schema) + `}}}`)
			issues := Check(doc)
			if (len(issues) == 0) != tc.Valid {
				t.Fatalf("valid=%v，diagnostics=%+v", tc.Valid, issues)
			}
		})
	}
}

// 极端指数不按位数展开，整数判定仍保持十进制精度。
func TestSchemaNumberTraits(t *testing.T) {
	for _, tc := range []struct {
		keyword, number string
		valid           bool
	}{
		{"minLength", "1e999999999999999999999999", true},
		{"minLength", "1e-999999999999999999999999", false},
		{"minLength", "0e-999999999999999999999999", true},
		{"minLength", "-0.0", true}, {"minLength", "100.00e-2", true},
		{"minLength", "100.00e-3", false}, {"minLength", "-1e999999999999999999999999", false},
		{"multipleOf", "1e-999999999999999999999999", true}, {"multipleOf", "-0e999999999999999999999999", false},
	} {
		t.Run(tc.keyword+tc.number, func(t *testing.T) {
			raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"` + tc.keyword + `":` + tc.number + `}}}}`)
			if issues := Check(raw); (len(issues) == 0) != tc.valid {
				t.Fatalf("valid=%v，issues=%+v", tc.valid, issues)
			}
		})
	}
}

// 组合成员仍是独立 Schema 位置，其相对引用和资源身份必须参与解析。
func TestSchemaArrayResourceReferences(t *testing.T) {
	good := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"allOf":[{"$id":"https://example.test/a","$anchor":"item","type":"string"},{"$ref":"https://example.test/a#item"}]}}}}`)
	if issues := Check(good); len(issues) != 0 {
		t.Fatalf("组合资源引用失效：%+v", issues)
	}
	bad := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":{"anyOf":[{"$ref":"file:///denied"}]}}}}`)
	if issues := Check(bad); len(issues) == 0 {
		t.Fatal("组合中的外部引用被忽略")
	}
}
