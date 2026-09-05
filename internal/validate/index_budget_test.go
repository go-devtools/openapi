package validate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// 长前缀不能通过大量短子字段放大索引；放宽预算后同一规范应可检查。
// Long prefixes must not amplify indexes through many short children; a larger budget accepts the same document.
func TestReferenceIndexTextBudget(t *testing.T) {
	properties := map[string]any{}
	for i := 0; i < 100; i++ {
		properties[fmt.Sprint(i)] = map[string]any{"type": "string"}
	}
	raw, err := json.Marshal(map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "a", "version": "1"}, "components": map[string]any{"schemas": map[string]any{strings.Repeat("p", 1024): map[string]any{"type": "object", "properties": properties}}}})
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckWithOptions(raw, Options{MaxIndexBytes: 64 << 10}); len(issues) == 0 {
		t.Fatal("索引文本未受预算限制")
	}
	if issues := CheckWithOptions(raw, Options{MaxIndexBytes: 256 << 10}); len(issues) != 0 {
		t.Fatal(issues)
	}
}

// 规范化的绝对引用不能无限复制长资源 URI。
// Normalized absolute references must not duplicate long resource URIs without a bound.
func TestNormalizedReferenceBudget(t *testing.T) {
	refs := make([]any, 100)
	for i := range refs {
		refs[i] = map[string]any{"$ref": "#/$defs/value"}
	}
	raw, err := json.Marshal(map[string]any{"$id": "https://example.test/" + strings.Repeat("p", 3000), "$defs": map[string]any{"value": map[string]any{"type": "string"}}, "allOf": refs})
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := BundleSchemas(raw, "", Options{MaxNormalizedBytes: 64 << 10}); len(issues) == 0 {
		t.Fatal("引用规范化结果未受预算限制")
	}
	if _, issues := BundleSchemas(raw, "", Options{MaxNormalizedBytes: 512 << 10}); len(issues) != 0 {
		t.Fatal(issues)
	}
}
