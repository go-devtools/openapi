package compiler

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/openapi-golang/openapi/internal/comment"
	"github.com/openapi-golang/openapi/spec"
)

// 将声明检查推迟到完整组件图建立后，避免漏掉命名类型和递归引用。
// Defer annotation checks until the component graph is complete to include named types and recursion.
type annotationCheck struct {
	schema *spec.Schema
	before *spec.Schema
	doc    comment.Document
	site   string
}

// 固定指令键顺序，诊断不依赖 Go map 的迭代次序。
// Sort directive keys so diagnostics do not depend on Go map iteration order.
func directiveKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// 只检查投影自身的类型及约束，不进入属性、数组元素或示例数据。
// Inspect only the projection's own type and constraints, excluding properties, items, and example data.
func (p *projector) annotationFacts(schema *spec.Schema) ([]*spec.SchemaObject, map[string]bool) {
	var facts []*spec.SchemaObject
	types := map[string]bool{}
	seen := map[*spec.Schema]bool{}
	var visit func(*spec.Schema)
	visit = func(s *spec.Schema) {
		if s == nil || seen[s] {
			return
		}
		seen[s] = true
		if s.Bool != nil {
			if *s.Bool {
				types["*"] = true
			}
			return
		}
		if s.SchemaObject == nil {
			types["*"] = true
			return
		}
		if len(s.Type) == 0 && s.Ref == "" && len(s.AllOf) == 0 && len(s.AnyOf) == 0 && len(s.OneOf) == 0 {
			types["*"] = true
		}
		facts = append(facts, s.SchemaObject)
		for _, kind := range s.Type {
			types[kind] = true
		}
		if strings.HasPrefix(s.Ref, "#/components/schemas/") {
			visit(p.components[strings.TrimPrefix(s.Ref, "#/components/schemas/")])
		}
		for _, part := range s.AllOf {
			visit(part)
		}
		for _, parts := range [][]*spec.Schema{s.AnyOf, s.OneOf} {
			// 标准投影用联合表达 null；复杂分支的业务约束交由集中扩展和契约验证。
			// Represent null with unions; delegate complex branch constraints to centralized extensions and contract validation.
			var nonnull []*spec.Schema
			for _, part := range parts {
				if part != nil && part.SchemaObject != nil && len(part.Type) == 1 && part.Type[0] == "null" {
					types["null"] = true
				} else {
					nonnull = append(nonnull, part)
				}
			}
			if len(nonnull) == 1 {
				visit(nonnull[0])
			} else if len(nonnull) > 1 {
				types["*"] = true
			}
		}
	}
	visit(schema)
	return facts, types
}

// 在组件图完整后检查源码声明与真实类型、派生边界的冲突。
// Check source declarations against actual types and derived bounds after building the component graph.
func (p *projector) checkAnnotations() error {
	for _, check := range p.annotations {
		if err := p.checkAnnotation(check); err != nil {
			return fmt.Errorf("%s: %w", check.site, err)
		}
	}
	return nil
}

// 约束只能施加于相应的网络类型，不能覆盖固定数组和数值编码事实。
// Apply constraints only to matching wire types without overriding fixed-array lengths or numeric encoding facts.
func (p *projector) checkAnnotation(check annotationCheck) error {
	facts, kinds := p.annotationFacts(check.schema)
	for _, directive := range check.doc.Directives {
		for _, key := range directiveKeys(directive.Values) {
			want := ""
			switch key {
			case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf":
				want = "number"
			case "minLength", "maxLength", "pattern", "contentEncoding", "contentMediaType":
				want = "string"
			case "minItems", "maxItems", "uniqueItems", "minContains", "maxContains":
				want = "array"
			case "minProperties", "maxProperties":
				want = "object"
			}
			if want != "" && !kinds["*"] && !kinds[want] && !(want == "number" && kinds["integer"]) {
				return fmt.Errorf("openapi.comment.type: %s 不适用于实际网络类型", key)
			}
		}
	}
	before := check.before
	if before != nil && before.SchemaObject != nil {
		after := check.schema
		if before.MinItems.Present && before.MaxItems.Present && before.MinItems.Value == before.MaxItems.Value {
			if !after.MinItems.Present || !after.MaxItems.Present || after.MinItems.Value != before.MinItems.Value || after.MaxItems.Value != before.MaxItems.Value {
				return fmt.Errorf("openapi.comment.derived: 注释不能改变固定数组的实际长度")
			}
		}
		if before.Minimum.Present && after.Minimum.Present {
			order, err := annotationNumberCompare(after.Minimum.Value, before.Minimum.Value)
			if err != nil {
				return err
			}
			if order < 0 {
				return fmt.Errorf("openapi.comment.derived: minimum 不能放宽类型派生的数值下界")
			}
		}
	}
	for _, pair := range [][2]string{{"minLength", "maxLength"}, {"minItems", "maxItems"}, {"minContains", "maxContains"}, {"minProperties", "maxProperties"}, {"minimum", "maximum"}, {"minimum", "exclusiveMaximum"}, {"exclusiveMinimum", "maximum"}, {"exclusiveMinimum", "exclusiveMaximum"}} {
		var lower, upper *json.Number
		for _, fact := range facts {
			a, b := annotationBound(fact, pair[0]), annotationBound(fact, pair[1])
			if a != nil {
				if lower == nil {
					lower = a
				} else {
					cmp, err := annotationNumberCompare(*a, *lower)
					if err != nil {
						return err
					}
					if cmp > 0 {
						lower = a
					}
				}
			}
			if b != nil {
				if upper == nil {
					upper = b
				} else {
					cmp, err := annotationNumberCompare(*b, *upper)
					if err != nil {
						return err
					}
					if cmp < 0 {
						upper = b
					}
				}
			}
		}
		if lower != nil && upper != nil {
			cmp, err := annotationNumberCompare(*lower, *upper)
			if err != nil {
				return err
			}
			exclusive := strings.HasPrefix(pair[0], "exclusive") || strings.HasPrefix(pair[1], "exclusive")
			if cmp > 0 || (cmp == 0 && exclusive) {
				return fmt.Errorf("openapi.comment.range: %s 与 %s 不能同时满足", pair[0], pair[1])
			}
		}
	}
	return nil
}

// 从已类型化的投影中读取边界，整数不经过浮点转换。
// Read bounds from typed projections without converting integers through floating point.
func annotationBound(s *spec.SchemaObject, key string) *json.Number {
	var integer spec.Optional[uint64]
	var number spec.Optional[json.Number]
	switch key {
	case "minLength":
		integer = s.MinLength
	case "maxLength":
		integer = s.MaxLength
	case "minItems":
		integer = s.MinItems
	case "maxItems":
		integer = s.MaxItems
	case "minContains":
		integer = s.MinContains
	case "maxContains":
		integer = s.MaxContains
	case "minProperties":
		integer = s.MinProperties
	case "maxProperties":
		integer = s.MaxProperties
	case "minimum":
		number = s.Minimum
	case "maximum":
		number = s.Maximum
	case "exclusiveMinimum":
		number = s.ExclusiveMinimum
	case "exclusiveMaximum":
		number = s.ExclusiveMaximum
	}
	if integer.Present {
		value := json.Number(strconv.FormatUint(integer.Value, 10))
		return &value
	}
	if number.Present {
		return &number.Value
	}
	return nil
}

// 编译期声明比较使用有界精确算术；极端指数返回预算诊断，不展开无限大整数。
// Use bounded exact arithmetic for annotation comparisons; report extreme exponents without expanding huge integers.
func annotationNumberCompare(a, b json.Number) (int, error) {
	values := []*big.Rat{}
	for _, number := range []json.Number{a, b} {
		text := string(number)
		if len(text) > 4096 {
			return 0, fmt.Errorf("openapi.comment.budget: 数值边界比较最多支持 4096 位文本")
		}
		if index := strings.IndexAny(text, "eE"); index >= 0 {
			exponent, err := strconv.Atoi(text[index+1:])
			if err != nil || exponent > 4096 || exponent < -4096 {
				return 0, fmt.Errorf("openapi.comment.budget: 数值边界比较的指数绝对值不能超过 4096")
			}
		}
		value, ok := new(big.Rat).SetString(text)
		if !ok {
			return 0, fmt.Errorf("openapi.comment.value: 数值边界不是合法 JSON 数字")
		}
		values = append(values, value)
	}
	return values[0].Cmp(values[1]), nil
}
