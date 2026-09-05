package openapi

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/openapi-golang/openapi/spec"
)

// 根据实际引用闭包保留完整组件，示例数据不能让无关模型变成可达。
// Retain complete reachable components without allowing example data to make unrelated models reachable.
func TestBuildPrunesWithSchemaResourceSemantics(t *testing.T) {
	cases := []struct {
		name, schemas, ref string
		want               []string
	}{
		{"resource-id", `{"Entry":{"$id":"https://example.test/entry","$ref":"value"},"Value":{"$id":"https://example.test/value","type":"string"},"Unused":true}`, "https://example.test/entry", []string{"Entry", "Value"}},
		{"anchor", `{"Entry":{"$anchor":"entry","properties":{"next":{"$ref":"#entry"}}},"Unused":true}`, "#entry", []string{"Entry"}},
		{"subschema-target", `{"Entry":{"$id":"https://example.test/entry","$defs":{"Part":{"$anchor":"part","type":"string"}},"properties":{"value":{"$ref":"https://example.test/value"}}},"Value":{"$id":"https://example.test/value","type":"integer"},"Unused":true}`, "https://example.test/entry#part", []string{"Entry", "Value"}},
		{"example-data", `{"Entry":{"examples":[{"$ref":"#/components/schemas/Unused","$dynamicRef":"https://data.invalid/ignored"}]},"Unused":true}`, "#/components/schemas/Entry", []string{"Entry"}},
		{"discriminator", `{"Entry":{"type":"object","discriminator":{"propertyName":"kind","mapping":{"cat":"Cat"},"defaultMapping":"Dog"}},"Cat":{"type":"object"},"Dog":{"type":"object"},"Unused":true}`, "#/components/schemas/Entry", []string{"Cat", "Dog", "Entry"}},
		{"discriminator-resource-name", `{"Entry":{"type":"object","discriminator":{"propertyName":"kind","mapping":{"cat":"Cat"},"defaultMapping":"Dog"}},"Cat":{"$id":"https://example.test/cat","type":"object"},"Dog":{"$id":"https://example.test/dog","type":"object"},"Unused":true}`, "#/components/schemas/Entry", []string{"Cat", "Dog", "Entry"}},
		{"unused-invalid-ref", `{"Entry":{"type":"string"},"Unused":{"$ref":"https://not-provided.invalid/schema"}}`, "#/components/schemas/Entry", []string{"Entry"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := testBundle(t).Snapshot()
			if err := json.Unmarshal([]byte(tc.schemas), &data.Components.Schemas); err != nil {
				t.Fatal(err)
			}
			data.Templates[0].Operation.Responses["200"].Value.Content["application/json"].Value.Schema = &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: tc.ref}}
			bundle, err := NewBundle(data)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Build(bundle, []Route{{Method: "GET", Path: "/sample", OperationKey: "user"}}, Config{Title: "sample", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			var parsed spec.OpenAPI
			if err = json.Unmarshal(doc.JSON(), &parsed); err != nil {
				t.Fatal(err)
			}
			var names []string
			for name := range parsed.Components.Schemas {
				names = append(names, name)
			}
			sort.Strings(names)
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("组件闭包错误：%v，预期 %v", names, tc.want)
			}
			if report := Check(doc.JSON()); report.HasErrors() {
				t.Fatal(report)
			}
		})
	}
}

// 外部 Schema 回指本地模型时，构建和裁剪必须使用同一组离线配置。
// Use identical offline options for building and pruning external schemas that refer back to local models.
func TestBuildOfflineResourceBackReference(t *testing.T) {
	data := testBundle(t).Snapshot()
	data.Templates[0].Operation.Responses["200"].Value.Content["application/json"].Value.Schema = &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "https://example.test/external"}}
	bundle, err := NewBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	options := CheckOptions{BaseURI: "https://example.test/openapi.json", Resources: map[string][]byte{
		"https://example.test/external": []byte(`{"properties":{"local":{"$ref":"https://example.test/openapi.json#/components/schemas/User"}}}`),
	}}
	cfg := Config{Title: "sample", Version: "1", Validation: options}
	routes := []Route{{Method: "GET", Path: "/sample", OperationKey: "user"}}
	doc, err := Build(bundle, routes, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed spec.OpenAPI
	if err = json.Unmarshal(doc.JSON(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Components.Schemas) != 1 || parsed.Components.Schemas["User"] == nil {
		t.Fatal("外部回指模型被误删")
	}
	if report := CheckWithOptions(doc.JSON(), options); report.HasErrors() {
		t.Fatal(report)
	}
	cfg.Validation.MaxReferences = 1
	if _, err = Build(bundle, routes, cfg); err == nil {
		t.Fatal("构建忽略引用预算")
	}
	cfg.Validation = CheckOptions{}
	if _, err = Build(bundle, routes, cfg); err == nil {
		t.Fatal("构建在缺少显式资源时仍成功")
	}
}

// 作为 externalValue 内容的已识别 Schema 资源也必须在裁剪后存在。
// Retain recognized schema resources used as externalValue contents after pruning.
func TestBuildKeepsSchemaUsedAsExampleResource(t *testing.T) {
	data := testBundle(t).Snapshot()
	data.Components.Schemas["User"].ID = "https://example.test/schema-example"
	data.Components.Examples = map[string]spec.RefOr[spec.Example]{"Schema": spec.Inline(spec.Example{ExternalValue: "https://example.test/schema-example"})}
	data.Templates[0].Operation.Responses["200"].Value.Content["application/json"].Value.Schema = spec.Typed("string")
	bundle, err := NewBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Build(bundle, []Route{{Method: "GET", Path: "/sample", OperationKey: "user"}}, Config{Title: "sample", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var parsed spec.OpenAPI
	if err = json.Unmarshal(doc.JSON(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Components.Schemas) != 1 || parsed.Components.Schemas["User"] == nil {
		t.Fatal("示例资源在裁剪时丢失")
	}
}
