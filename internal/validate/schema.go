package validate

import (
	"encoding/json"
	"strings"
)

// Check standard keyword shapes; inapplicable constraints and unsatisfiable schemas remain valid specifications.
func (c *checker) schema(m map[string]any, path string) {
	if has(m, "nullable") {
		c.add("schema.nullable", path+"/nullable", "use a null union instead of the legacy nullable keyword")
	}
	for _, key := range sortedKeys(m) {
		if c.stopped() {
			return
		}
		value := m[key]
		valid := true
		switch key {
		case "$schema":
			text, ok := value.(string)
			valid = ok
			if valid {
				uri, err := parseURIReference(text)
				valid = err == nil && uri.IsAbs()
			}
		case "type":
			if text, ok := value.(string); ok {
				valid = simpleType(text)
			} else {
				list, ok := value.([]any)
				valid = ok && len(list) > 0 && uniqueStrings(list)
				if valid {
					for _, item := range list {
						if c.stopped() {
							return
						}
						valid = valid && simpleType(item.(string))
					}
				}
			}
		case "enum", "examples":
			_, valid = value.([]any)
		case "title", "description", "$comment", "pattern", "format", "contentEncoding", "contentMediaType":
			_, valid = value.(string)
		case "readOnly", "writeOnly", "deprecated", "uniqueItems":
			_, valid = value.(bool)
		case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf":
			number, ok := value.(json.Number)
			valid = ok
			if valid && key == "multipleOf" {
				negative, zero, _ := numberTraits(number)
				valid = !negative && !zero
			}
		case "minLength", "maxLength", "minItems", "maxItems", "minContains", "maxContains", "minProperties", "maxProperties":
			number, ok := value.(json.Number)
			valid = ok
			if valid {
				negative, zero, integer := numberTraits(number)
				valid = integer && (!negative || zero)
			}
		case "required":
			list, ok := value.([]any)
			valid = ok && uniqueStrings(list)
		case "dependentRequired":
			entries, ok := value.(map[string]any)
			valid = ok
			if ok {
				for _, name := range sortedKeys(entries) {
					if c.stopped() {
						return
					}
					list, ok := entries[name].([]any)
					if !ok || !uniqueStrings(list) {
						c.add("schema.keyword", path+"/dependentRequired/"+escape(name), "Dependencies must be string arrays without duplicates")
					}
				}
			}
		}
		if !valid {
			c.add("schema.keyword", path+"/"+escape(key), key+" does not conform to JSON Schema 2020-12")
		}
	}
}

// Recognize the seven standard type names.
func simpleType(value string) bool {
	switch value {
	case "null", "boolean", "object", "array", "number", "string", "integer":
		return true
	}
	return false
}

// Allow empty arrays and require nonempty entries to be unique strings.
func uniqueStrings(values []any) bool {
	seen := map[string]bool{}
	for _, value := range values {
		text, ok := value.(string)
		if !ok || seen[text] {
			return false
		}
		seen[text] = true
	}
	return true
}

// Recognize signs, zero, and integers from decimal text without exponent expansion or floating-point conversion.
func numberTraits(number json.Number) (negative, zero, integer bool) {
	text := string(number)
	negative = strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	mantissa, exponent := text, "0"
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		mantissa, exponent = text[:i], text[i+1:]
	}
	fraction := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fraction = len(mantissa) - i - 1
		mantissa = mantissa[:i] + mantissa[i+1:]
	}
	significant := strings.TrimRight(mantissa, "0")
	if significant == "" {
		return negative, true, true
	}
	required := fraction - (len(mantissa) - len(significant))
	sign := 1
	if strings.HasPrefix(exponent, "-") {
		sign = -1
	}
	exponent = strings.TrimLeft(exponent, "+-")
	// Bound classification thresholds by input length and continue scanning after saturation without exponent-sized allocations.
	limit := len(text) + 1
	value := 0
	for _, digit := range exponent {
		if value < limit {
			value = value*10 + int(digit-'0')
			if value > limit {
				value = limit
			}
		}
	}
	return negative, false, sign*value >= required
}

// Check a schema object's keyword values before typed decoding without resolving references or testing satisfiability.
func SchemaKeywordValues(object map[string]any) []Issue {
	c := checker{}
	c.schema(object, "#")
	return c.issues
}
