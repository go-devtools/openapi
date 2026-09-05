package compiler_test

import (
	"encoding/json"
	"testing"

	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
)

// 非法投影必须在导出时失败，不能生成含 null Schema 或悬空引用的文件。
// Reject invalid projections during export instead of emitting null schemas or dangling references.
func TestStandaloneRejectsInvalidProjection(t *testing.T) {
	cases := map[string]*compiler.Projection{
		"nil projection":    nil,
		"nil root":          {},
		"nil component":     {Root: spec.Typed("string"), Components: map[string]*spec.Schema{"Missing": nil}},
		"missing component": {Root: &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/Missing"}}},
		"missing anchor":    {Root: &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#missing"}}},
	}
	for name, projection := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if failure := recover(); failure != nil {
					t.Errorf("export panicked: %v", failure)
				}
			}()
			raw, err := projection.Standalone()
			if err == nil || len(raw) != 0 {
				t.Fatalf("invalid projection exported: %s; error: %v", raw, err)
			}
		})
	}
}

// 显式方言不能被默认值覆盖，调用方应能识别实际导出的 Schema 方言。
// Never overwrite an explicit dialect with a default; consumers must see the actual schema dialect.
func TestStandalonePreservesDeclaredDialect(t *testing.T) {
	root := spec.Typed("string")
	root.Schema = "https://example.test/meta"
	raw, err := (&compiler.Projection{Root: root}).Standalone()
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value["$schema"] != root.Schema {
		t.Fatalf("explicit dialect was changed: %s", raw)
	}
}
