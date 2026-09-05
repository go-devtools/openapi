package contracttest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// 在父级资源作用域中编译嵌套指针，不能丢失最近的标识符与锚点。
// Compile nested pointers within their parent resource without losing the nearest identifier or anchor.
func TestNestedSchemaResourceScope(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","components":{"schemas":{"Product":{"$id":"https://example.test/product","$defs":{"Name":{"$anchor":"name","type":"string","minLength":3}},"type":"object","properties":{"name":{"$ref":"#name"}}}}}}`)
	v, err := Compile(raw, "/components/schemas/Product/properties/name", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"alice"`)); err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"ab"`)); err == nil {
		t.Fatal("未检查父资源中的目标约束")
	}
}

// OpenAPI 中没有独立标识符的组件共享文档锚点作用域。
// Components without their own identifiers share the OpenAPI document's anchor scope.
func TestOpenAPISharedAnchors(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","$self":"https://example.test/api/openapi","components":{"schemas":{"A":{"$ref":"#name"},"B":{"$anchor":"name","type":"string","const":"yes"}}}}`)
	v, err := Compile(raw, "/components/schemas/A", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"yes"`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`"no"`)) == nil {
		t.Fatal("共享锚点丢失约束")
	}
}

// 动态递归必须让扩展资源约束子节点，普通引用则继续使用基础资源。
// Dynamic recursion applies extension constraints to children while static references keep the base resource.
func TestDynamicReferenceInstances(t *testing.T) {
	document := `{"openapi":"3.2.0","components":{"schemas":{
 "Tree":{"$id":"https://example.test/tree","$dynamicAnchor":"node","type":"object","properties":{"data":true,"children":{"type":"array","items":{"$dynamicRef":"#node"}}}},
 "Strict":{"$id":"https://example.test/strict","$dynamicAnchor":"node","$ref":"https://example.test/tree","unevaluatedProperties":false}
 }}}`
	strict, err := Compile([]byte(document), "/components/schemas/Strict", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = strict.JSON([]byte(`{"data":1,"children":[{"data":2}]}`)); err != nil {
		t.Fatal(err)
	}
	if strict.JSON([]byte(`{"children":[{"data":2,"extra":true}]}`)) == nil {
		t.Fatal("动态引用未继承递归扩展约束")
	}
	for _, variant := range []string{
		strings.Replace(document, `"$dynamicAnchor":"node"`, `"$anchor":"node"`, 1),
		strings.ReplaceAll(document, `"$dynamicRef":"#node"`, `"$dynamicRef":"#"`),
	} {
		fallback, err := Compile([]byte(variant), "/components/schemas/Strict", Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err = fallback.JSON([]byte(`{"children":[{"extra":true}]}`)); err != nil {
			t.Fatal("非动态初始目标必须保持静态语义", err)
		}
	}
	base, err := Compile([]byte(document), "/components/schemas/Tree", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = base.JSON([]byte(`{"children":[{"extra":true}]}`)); err != nil {
		t.Fatal("基础资源被扩展污染", err)
	}
	static, err := Compile([]byte(strings.ReplaceAll(document, `"$dynamicRef":"#node"`, `"$ref":"#node"`)), "/components/schemas/Strict", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = static.JSON([]byte(`{"children":[{"extra":true}]}`)); err != nil {
		t.Fatal("静态引用被误当成动态引用", err)
	}
}

// 样例数据中的引用和资源字段不参与 Schema 索引。
// Resource and reference fields inside example data do not participate in schema indexing.
func TestContractIgnoresExampleResourceFields(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","components":{"schemas":{"X":{"type":"string","examples":[{"$id":"https://example.test/x","$ref":"file:///denied"}]}}}}`)
	v, err := Compile(raw, "/components/schemas/X", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"yes"`)); err != nil {
		t.Fatal(err)
	}
}

// 显式检索地址、相对 self/id 与预载资源共同决定引用目标。
// Explicit retrieval addresses, relative self/id values, and preloaded resources determine reference targets.
func TestContractExplicitOfflineResources(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","$self":"/api/root","components":{"schemas":{"Container":{"$id":"schemas/container","properties":{"item":{"$ref":"item#item"}}}}}}`)
	resource := []byte(`{"$id":"./canonical-item","$anchor":"item","type":"integer","minimum":3}`)
	options := Options{BaseURI: "https://example.test/staging/root", Resources: map[string][]byte{"https://example.test/api/schemas/item": resource}}
	original := bytes.Clone(resource)
	v, err := Compile(raw, "/components/schemas/Container", options)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`{"item":3}`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`{"item":2}`)) == nil {
		t.Fatal("跨资源边界丢失")
	}
	if !bytes.Equal(resource, original) {
		t.Fatal("预载字节被修改")
	}
	options.Resources["https://example.test/api/schemas/item"] = []byte(`false`)
	if err = v.JSON([]byte(`{"item":3}`)); err != nil {
		t.Fatal("编译结果保留了可变的调用方资源", err)
	}
}

// 外部 OpenAPI 组件、布尔 Schema 和调用方明确提供的元 Schema 都保持离线。
// External OpenAPI components, boolean schemas, and explicitly supplied meta-schemas stay offline.
func TestContractResourceKindsAndDialect(t *testing.T) {
	options := Options{Resources: map[string][]byte{
		"https://example.test/shared": []byte(`{"openapi":"3.2.0","components":{"schemas":{"Name":{"type":"string","minLength":3}}}}`),
		"https://example.test/no":     []byte(`false`),
	}}
	v, err := Compile([]byte(`{"anyOf":[{"$ref":"https://example.test/shared#/components/schemas/Name"},{"$ref":"https://example.test/no"}]}`), "", options)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"alice"`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`"ab"`)) == nil {
		t.Fatal("外部 OpenAPI 组件约束丢失")
	}
	for _, dialect := range []string{"https://spec.openapis.org/oas/3.1/dialect/base", "https://spec.openapis.org/oas/3.2/dialect/2025-09-17"} {
		if _, err = Compile([]byte(`{"$schema":"`+dialect+`","type":"string"}`), "", Options{}); err != nil {
			t.Fatal(err)
		}
	}
	options.Resources["https://example.test/dialect"] = []byte(`{"$id":"https://example.test/dialect","$schema":"https://json-schema.org/draft/2020-12/schema","$vocabulary":{"https://json-schema.org/draft/2020-12/vocab/validation":true},"type":["object","boolean"]}`)
	if _, err = Compile([]byte(`{"$schema":"https://example.test/dialect","type":"integer"}`), "", options); err != nil {
		t.Fatal(err)
	}
	options.Resources["https://example.test/dialect"] = []byte(`{"$id":"https://example.test/dialect","$schema":"https://json-schema.org/draft/2020-12/schema","$vocabulary":{"https://example.test/unknown-vocabulary":true},"type":["object","boolean"]}`)
	if _, err = Compile([]byte(`{"$schema":"https://example.test/dialect"}`), "", options); err == nil {
		t.Fatal("未拒绝未知必需词汇表")
	}
}

// 缺失资源不会引发 HTTP 请求，预载同一 URI 后仍不会访问服务器。
// Missing resources never trigger HTTP requests; preloading the same URI also avoids server access.
func TestContractNeverFetches(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); _, _ = w.Write([]byte(`false`)) }))
	defer server.Close()
	raw, _ := json.Marshal(map[string]any{"$ref": server.URL + "/schema"})
	if _, err := Compile(raw, "", Options{}); err == nil {
		t.Fatal("缺少资源应失败")
	}
	if _, err := Compile(raw, "", Options{Resources: map[string][]byte{server.URL + "/schema": []byte(`true`)}}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatal("校验器发起了网络请求")
	}
}

// 规范位置、身份冲突和累计资源预算必须在独立引擎编译前检查。
// Schema locations, identity conflicts, and aggregate resource budgets are checked before independent compilation.
func TestContractResourceErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		pointer string
		options Options
	}{
		{"错误位置", `{"openapi":"3.2.0","info":{"title":"a"}}`, "/info", Options{}},
		{"示例数据", `{"type":"string","examples":[{}]}`, "/examples/0", Options{}},
		{"非法转义", `{"type":"object"}`, "/~invalid", Options{}},
		{"重复键", `{"type":"string","type":"integer"}`, "", Options{}},
		{"预载重复键", `true`, "", Options{Resources: map[string][]byte{"https://example.test/a": []byte(`{"type":"string","type":"integer"}`)}}},
		{"累计字节", `true`, "", Options{MaxBytes: 8, Resources: map[string][]byte{"https://example.test/a": []byte(`false`)}}},
		{"内嵌资源", `{"$defs":{"a":{"$id":"https://example.test/a"}}}`, "", Options{MaxResources: 1}},
		{"引用预算", `{"allOf":[{"$ref":"#"},{"$ref":"#"}]}`, "", Options{MaxReferences: 1}},
		{"重复标识", `{"$defs":{"a":{"$id":"https://example.test/a"},"b":{"$id":"https://example.test/a"}}}`, "", Options{}},
		{"跨越标识", `{"$ref":"#/$defs/a","$defs":{"a":{"$id":"https://example.test/a","type":"string"}}}`, "", Options{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compile([]byte(tc.raw), tc.pointer, tc.options); err == nil {
				t.Fatal("非法输入被接受")
			}
		})
	}
}

// 独立数值引擎前的预算检查阻止指数展开，并保护已解码样本的递归深度。
// Budgets before the independent numeric engine prevent exponent expansion and bound decoded sample depth.
func TestContractEngineBudgets(t *testing.T) {
	if _, err := Compile([]byte(`{"minimum":1e5000}`), "", Options{}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("需要数值预算诊断：%v", err)
	}
	v, err := Compile([]byte(`true`), "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`1e999999999999999999`)); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("需要样本数值预算诊断：%v", err)
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if err = v.Value(cyclic); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("需要样本深度预算诊断：%v", err)
	}
	small, err := Compile([]byte(`true`), "", Options{MaxBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	if err = small.Value("abcdef"); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("需要内容字节预算诊断：%v", err)
	}
}
