package contracttest

import (
	"encoding/json"
	"strings"
	"testing"
)

// The public API forwards both budgets and validates real instances when given sufficient limits.
// 公开入口必须透传索引与规范化预算，放宽预算后仍执行真实实例验证。
func TestResourceTextBudgets(t *testing.T) {
	refs := make([]any, 100)
	for i := range refs {
		refs[i] = map[string]any{"$ref": "#/$defs/value"}
	}
	raw, err := json.Marshal(map[string]any{"$id": "https://example.test/" + strings.Repeat("p", 3000), "$defs": map[string]any{"value": map[string]any{"type": "string"}}, "allOf": refs})
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []Options{{MaxNormalizedBytes: 64 << 10}, {MaxIndexBytes: 64 << 10}} {
		if _, err := Compile(raw, "", options); err == nil || !strings.Contains(err.Error(), "openapi.spec.budget") {
			t.Fatal(err)
		}
	}
	validator, err := Compile(raw, "", Options{MaxNormalizedBytes: 512 << 10, MaxIndexBytes: 2 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON([]byte(`"ok"`)); err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON([]byte(`1`)); err == nil {
		t.Fatal("number must not satisfy a string contract")
	}
}
