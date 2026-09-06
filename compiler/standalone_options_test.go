package compiler_test

import (
	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
	"testing"
)

// Require valid absolute dialect URIs so invalid declarations never enter standalone files.
// 方言必须是合法绝对 URI，非法声明不能进入独立文件。
func TestStandaloneRejectsInvalidDialect(t *testing.T) {
	for _, dialect := range []string{"relative/meta", "https://example.test/bad space", "https://example.test/%broken"} {
		root := spec.Typed("string")
		root.Schema = dialect
		raw, err := (&compiler.Projection{Root: root}).Standalone()
		if err == nil || len(raw) > 0 {
			t.Errorf("invalid dialect accepted: %q %s", dialect, raw)
		}
	}
}

// Preserve the accept-all or reject-all semantics of boolean roots.
// 布尔根保留接受或拒绝所有实例的真实语义。
func TestStandaloneBooleanRoots(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		raw, err := (&compiler.Projection{Root: spec.Boolean(allowed)}).Standalone()
		if err != nil {
			t.Fatal(err)
		}
		validator := compileStandalone(t, raw)
		for _, value := range []any{nil, "text", 12, map[string]any{}} {
			if (validator.Validate(value) == nil) != allowed {
				t.Fatalf("boolean=%v value=%v", allowed, value)
			}
		}
	}
}

// Existing local definitions cannot stand in for missing projection components.
// 自有 $defs 不能冒充未提供的投影组件。
func TestStandaloneComponentNamesAreExplicit(t *testing.T) {
	root := &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/Local", Defs: map[string]*spec.Schema{"Local": spec.Typed("string")}}}
	if raw, err := (&compiler.Projection{Root: root}).Standalone(); err == nil || len(raw) > 0 {
		t.Fatalf("missing component rebound to local definition: %s", raw)
	}
}
