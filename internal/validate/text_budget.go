package validate

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// Distinguishes excessive index input from invalid options.
var errIndexBudget = errors.New("index and diagnostic text exceed the cumulative byte budget")

// Follows the current encoder's invalid UTF-8 replacement using one fixed probe.
var invalidUTF8JSONBytes = func() int {
	raw, _ := json.Marshal("\xff")
	return len(raw) - 2
}()

// Accounts for cumulative processed bytes, not exact Go heap allocations.
type byteBudget struct {
	remaining int
	exceeded  bool
}

// Checks before subtraction to avoid overflow and repeated work after exhaustion.
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
func (g *referenceGraph) childPath(parent, key string) string {
	if !g.spend(len(parent), 1, len(key), strings.Count(key, "~"), strings.Count(key, "/")) {
		return ""
	}
	return parent + "/" + escape(key)
}

// Charges selected pointers so caller-supplied text cannot bypass index limits.
func (g *referenceGraph) pointerPath(resource *referenceNode, fragment string) (string, string) {
	if !g.spend(len(resource.path), len(fragment)) {
		return "", "budget"
	}
	return pointerPath(resource, fragment)
}

// Structural and reference checks share the exhaustion state.
func (c *checker) stopped() bool {
	return c.graph != nil && c.graph.budget.exceeded
}

// Measures strings using encoding/json's default escapes without allocating an encoded copy.
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
func (g *referenceGraph) location(parts ...string) string {
	for _, part := range parts {
		if !g.spend(len(part)) {
			return ""
		}
	}
	return strings.Join(parts, "")
}
