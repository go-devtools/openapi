package validate

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// Distinguishes excessive index input from invalid options.
// 区分索引输入超限与非法配置。
var errIndexBudget = errors.New("index and diagnostic text exceed the cumulative byte budget")

// Follows the current encoder's invalid UTF-8 replacement using one fixed probe.
// 跟随当前标准编码器的非法 UTF-8 替换形式，只测量一个固定字符串。
var invalidUTF8JSONBytes = func() int {
	raw, _ := json.Marshal("\xff")
	return len(raw) - 2
}()

// Accounts for cumulative processed bytes, not exact Go heap allocations.
// 按累计处理字节计费，不声称与 Go 堆分配字节完全相同。
type byteBudget struct {
	remaining int
	exceeded  bool
}

// Checks before subtraction to avoid overflow and repeated work after exhaustion.
// 在减法前检查上限，避免加法溢出和超限后的重复处理。
func (b *byteBudget) spend(sizes ...int) bool {
	if b.exceeded {
		return false
	}
	for _, size := range sizes {
		if size < 0 || size > b.remaining {
			b.exceeded = true
			return false
		}
		b.remaining -= size
	}
	return true
}

// Shares the index and diagnostic budget and emits one fixed-size exhaustion error.
// 共享索引和诊断预算；耗尽时只保留一个固定长度错误。
func (g *referenceGraph) spend(sizes ...int) bool {
	if g.budget.exceeded {
		return false
	}
	if g.budget.spend(sizes...) {
		return true
	}
	g.issues = append(g.issues, Issue{Code: "openapi.spec.budget", Path: "#", Message: "index and diagnostic text exceed the cumulative byte budget", Fix: "Simplify resources and references, or explicitly increase MaxIndexBytes"})
	return false
}

// Charges escaped paths before constructing them so repeated long prefixes stay bounded.
// 在构造转义路径之前计费，长前缀不能通过循环放大分配。
func (g *referenceGraph) childPath(parent, key string) string {
	if !g.spend(len(parent), 1, len(key), strings.Count(key, "~"), strings.Count(key, "/")) {
		return ""
	}
	return parent + "/" + escape(key)
}

// Charges selected pointers so caller-supplied text cannot bypass index limits.
// 选择指针也受索引预算限制，调用方提供的文本不绕过输入预算。
func (g *referenceGraph) pointerPath(resource *referenceNode, fragment string) (string, string) {
	if !g.spend(len(resource.path), len(fragment)) {
		return "", "budget"
	}
	return pointerPath(resource, fragment)
}

// Structural and reference checks share the exhaustion state.
// 结构检查与引用检查共享超限状态。
func (c *checker) stopped() bool {
	return c.graph != nil && c.graph.budget.exceeded
}

// Measures strings using encoding/json's default escapes without allocating an encoded copy.
// 不生成编码副本，按 encoding/json 默认转义计算字符串体积。
func (b *byteBudget) quoted(value string) bool {
	if !b.spend(2) {
		return false
	}
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		n := size
		switch r {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			n = 2
		case '<', '>', '&', '\u2028', '\u2029':
			n = 6
		default:
			if r < 0x20 {
				n = 6
			} else if r == utf8.RuneError && size == 1 {
				n = invalidUTF8JSONBytes
			}
		}
		if !b.spend(n) {
			return false
		}
		value = value[size:]
	}
	return true
}

// Measures decoded JSON values and charges separators and empty containers exactly.
// 只测量本包解码产生的 JSON 值，分隔符与空容器按实际编码计费。
func (b *byteBudget) jsonValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return b.spend(4)
	case bool:
		if v {
			return b.spend(4)
		}
		return b.spend(5)
	case string:
		return b.quoted(v)
	case json.Number:
		return b.spend(len(v))
	case []any:
		if !b.spend(2, max(0, len(v)-1)) {
			return false
		}
		for _, child := range v {
			if !b.jsonValue(child) {
				return false
			}
		}
		return true
	case map[string]any:
		if !b.spend(2, max(0, len(v)-1), len(v)) {
			return false
		}
		for key, child := range v {
			if !b.quoted(key) || !b.jsonValue(child) {
				return false
			}
		}
		return true
	default:
		b.exceeded = true
		return false
	}
}

// Adjusts only the encoded size delta when replacing a string, avoiding repeated document encoding.
// 替换字符串时仅调整编码体积差额，避免逐次重编码整个文档。
func (b *byteBudget) replaceString(object map[string]any, key, value string, limit int) bool {
	previous := byteBudget{remaining: limit}
	if !previous.jsonValue(object[key]) {
		return false
	}
	b.remaining += limit - previous.remaining
	if !b.quoted(value) {
		return false
	}
	object[key] = value
	return true
}

// Charges relocated location text before concatenation; the caller reports exhaustion.
// 在拼接转换后的位置文本前计费，超限状态由调用方统一返回。
func (g *referenceGraph) location(parts ...string) string {
	for _, part := range parts {
		if !g.spend(len(part)) {
			return ""
		}
	}
	return strings.Join(parts, "")
}
