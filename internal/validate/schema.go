package validate

import (
	"encoding/json"
	"strings"
)

// 检查标准关键字的取值形状；不适用的约束与不可满足的 Schema 仍属于合法规范。
func (c *checker) schema(m map[string]any, path string) {
	if has(m, "nullable") {
		c.add("schema.nullable", path+"/nullable", "应使用联合 null，不能输出旧版 nullable")
	}
	for _, key := range sortedKeys(m) {
		if c.stopped() {
			return
		}
		value := m[key]
		valid := true
		switch key {
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
						c.add("schema.keyword", path+"/dependentRequired/"+escape(name), "依赖项必须是没有重复元素的字符串数组")
					}
				}
			}
		}
		if !valid {
			c.add("schema.keyword", path+"/"+escape(key), key+" 的值不符合 JSON Schema 2020-12")
		}
	}
}

// 判断标准允许的七种类型名。
func simpleType(value string) bool {
	switch value {
	case "null", "boolean", "object", "array", "number", "string", "integer":
		return true
	}
	return false
}

// 空数组合法，非空项必须是互不重复的字符串。
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

// 按十进制文本识别符号、零和整数，不按指数展开大整数或转换为浮点数。
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
	// 判定阈值不超过输入长度；饱和后继续扫描，不产生指数相关分配。
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

// 在类型化解码前检查单个 Schema 对象的关键字值，不解析引用或判断实例可满足性。
func SchemaKeywordValues(object map[string]any) []Issue {
	c := checker{}
	c.schema(object, "#")
	return c.issues
}
