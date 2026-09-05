package contracttest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// 固定官方资源并离线编译完整三点二结构与 Schema 方言验证器。
// Pin official resources and compile full OAS 3.2 structure and dialect validators offline.
func official32(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(offlineLoader{})
	c.AssertFormat()
	checksums := map[string]string{
		"schema.json":      "7d48f01f37eeae4799041b371ad5f533f9f533fd2b0caa1011a8ba27c5b48b70",
		"schema-base.json": "423daa88e2285fa343856c08502fe63fd8aa3674cd5b4ef88746ba6f82647af3",
		"dialect.json":     "4e2c989f3d1e6489d41bc1ca4ade11743278c612b82fba9144c6116c79f1c273",
		"meta.json":        "a1959c0aa1f9a7ce58f2b75699be5bc5187c8a25901fe04a227f1d9419ba4e9c",
	}
	for name, sum := range checksums {
		raw, err := os.ReadFile(filepath.Join("testdata", "oas32", name))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != sum {
			t.Fatalf("上游资源校验和变化：%s", name)
		}
		v, err := decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.AddResource(v.(map[string]any)["$id"].(string), v); err != nil {
			t.Fatal(err)
		}
	}
	v, err := c.Compile("https://spec.openapis.org/oas/3.2/schema-base/2025-11-23")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// 同一完整正例分别破坏关键标准字段，证明独立校验实际拒绝错误。
// Mutate standard fields in the complete valid example to prove independent validation rejects errors.
func TestOfficialOpenAPI32Matrix(t *testing.T) {
	v := official32(t)
	raw, err := os.ReadFile("../testdata/golden/openapi32-full.json")
	if err != nil {
		t.Fatal(err)
	}
	full, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.Validate(full); err != nil {
		t.Fatalf("完整标准样例：%v", err)
	}
	cases := []struct {
		name   string
		path   []string
		field  string
		value  any
		remove bool
	}{
		{"旧规范版本", nil, "openapi", "3.1.0", false},

		{"空安全声明", nil, "security", nil, false},
		{"许可标识冲突", []string{"info", "license"}, "url", "https://example.test/license", false},
		{"整段查询缺名", []string{"components"}, "parameters", map[string]any{"Bad": map[string]any{"in": "querystring", "content": map[string]any{"application/json": map[string]any{"schema": true}}}}, false},
		{"流式 Schema 类型", []string{"components", "mediaTypes", "Events"}, "itemSchema", nil, false},
		{"媒体示例冲突", []string{"components", "mediaTypes", "Events"}, "example", "event", false},
		{"设备授权缺端点", []string{"components", "securitySchemes", "device", "flows", "deviceAuthorization"}, "tokenUrl", nil, true},
		{"错误 XML 节点", []string{"components", "schemas", "XmlItem", "xml"}, "nodeType", "unknown", false},
		{"XML 新旧表达冲突", []string{"components", "schemas", "XmlItem", "xml"}, "attribute", true, false},
		{"非法联合类型", []string{"components", "schemas", "Item"}, "type", []any{"object", "unknown"}, false},
		{"负长度", []string{"components", "schemas", "Constraints"}, "minLength", json.Number("-1"), false},
		{"空组合", []string{"components", "schemas", "Constraints"}, "anyOf", []any{}, false},
		{"重复必需属性", []string{"components", "schemas", "Item"}, "required", []any{"id", "id"}, false},
		{"零倍数", []string{"components", "schemas", "Constraints"}, "multipleOf", json.Number("0"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copy, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			node := copy.(map[string]any)
			for _, key := range tc.path {
				next, ok := node[key].(map[string]any)
				if !ok {
					t.Fatalf("测试路径不存在：%v，%s", tc.path, key)
				}
				node = next
			}
			if tc.remove {
				delete(node, tc.field)
			} else {
				node[tc.field] = tc.value
			}
			if err = v.Validate(copy); err == nil {
				t.Fatal("独立校验未拒绝错误样例")
			}
		})
	}
}
