package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/openapi-golang/openapi/spec"
)

// 描述有限的请求方法与媒体类型决策；包含集合为空表示该维度不受限。
// Describe finite method and media decisions; an empty inclusion set leaves that dimension unrestricted.
type RequestCondition struct {
	Methods          []string `json:"methods,omitempty"`
	ExceptMethods    []string `json:"exceptMethods,omitempty"`
	MediaTypes       []string `json:"mediaTypes,omitempty"`
	ExceptMediaTypes []string `json:"exceptMediaTypes,omitempty"`
}

// 保存已经投影的条件契约，不包含业务函数或分析期类型对象。
// Store a projected conditional contract without business functions or analysis-time type objects.
type OperationVariant struct {
	When        RequestCondition `json:"when"`
	Operation   spec.Operation   `json:"operation"`
	Diagnostics []Diagnostic     `json:"diagnostics,omitempty"`
	Facts       []Source         `json:"facts,omitempty"`
}

// 标识当前 Bundle 是否需要运行时条件选择能力。
// Identify the runtime capability required for conditional Bundle selection.
const RequestConditionsCapability = "request-conditions-v1"

// 判断条件是否适用于全部请求，不修改调用方集合。
// Report whether a condition applies to every request without changing caller-owned sets.
func (c RequestCondition) Unconditional() bool {
	return len(c.Methods)+len(c.ExceptMethods)+len(c.MediaTypes)+len(c.ExceptMediaTypes) == 0
}

// 规范化有限集合，拒绝无法执行的条件并报告空交集。
// Normalize finite sets, reject invalid conditions, and report empty intersections.
func (c RequestCondition) Intersect(other RequestCondition) (RequestCondition, bool, error) {
	a, ok, err := normalizeCondition(c)
	if err != nil || !ok {
		return a, ok, err
	}
	b, ok, err := normalizeCondition(other)
	if err != nil || !ok {
		return b, ok, err
	}
	methods, exceptMethods, ok := intersectDomain(a.Methods, a.ExceptMethods, b.Methods, b.ExceptMethods)
	if !ok {
		return RequestCondition{}, false, nil
	}
	media, exceptMedia, ok := intersectDomain(a.MediaTypes, a.ExceptMediaTypes, b.MediaTypes, b.ExceptMediaTypes)
	if !ok {
		return RequestCondition{}, false, nil
	}
	return RequestCondition{Methods: methods, ExceptMethods: exceptMethods, MediaTypes: media, ExceptMediaTypes: exceptMedia}, true, nil
}

// 用精确字符串匹配 HTTP 标记；媒体类型中的空字符串表示没有 Content-Type。
// Match exact HTTP tokens; an empty media string denotes an absent Content-Type.
func validConditionToken(value string) bool {
	if value == "" {
		return false
	}
	for _, b := range []byte(value) {
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))) {
			return false
		}
	}
	return true
}

// 将集合复制并排序，验证方法和媒体名称，不进行隐式大小写改写。
// Copy and sort sets, validating method and media names without implicit case rewriting.
func conditionSet(values []string, media bool) ([]string, error) {
	if len(values) > 256 {
		return nil, fmt.Errorf("openapi.condition.budget: 条件集合超过限制")
	}
	set := map[string]bool{}
	for _, value := range values {
		valid := validConditionToken(value)
		if media {
			parts := strings.Split(value, "/")
			valid = value == "" || len(parts) == 2 && validConditionToken(parts[0]) && validConditionToken(parts[1]) && !strings.Contains(value, "*")
		}
		if !valid {
			return nil, fmt.Errorf("openapi.condition.invalid: 非法条件值 %q", value)
		}
		set[value] = true
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

// 对单个条件消除重复和相互排除的成员，空可达集不能表示为无条件。
// Remove duplicates and exclusions without mistaking an empty reachable set for an unconditional condition.
func normalizeCondition(c RequestCondition) (RequestCondition, bool, error) {
	lists := []*[]string{&c.Methods, &c.ExceptMethods, &c.MediaTypes, &c.ExceptMediaTypes}
	for i, list := range lists {
		values, err := conditionSet(*list, i >= 2)
		if err != nil {
			return c, false, err
		}
		*list = values
	}
	var ok bool
	c.Methods, c.ExceptMethods, ok = intersectDomain(c.Methods, c.ExceptMethods, nil, nil)
	if !ok {
		return c, false, nil
	}
	c.MediaTypes, c.ExceptMediaTypes, ok = intersectDomain(c.MediaTypes, c.ExceptMediaTypes, nil, nil)
	return c, ok, nil
}

// 求两个包含/排除集合的交集，所有返回切片均为独立副本。
// Intersect inclusion/exclusion domains and return independently owned slices.
func intersectDomain(a, ax, b, bx []string) ([]string, []string, bool) {
	excluded := map[string]bool{}
	for _, v := range ax {
		excluded[v] = true
	}
	for _, v := range bx {
		excluded[v] = true
	}
	var allowed []string
	finite := len(a) > 0 || len(b) > 0
	if len(a) > 0 {
		for _, v := range a {
			if len(b) == 0 || stringMember(b, v) {
				if !excluded[v] {
					allowed = append(allowed, v)
				}
			}
		}
	} else {
		for _, v := range b {
			if !excluded[v] {
				allowed = append(allowed, v)
			}
		}
	}
	if finite {
		return allowed, nil, len(allowed) > 0
	}
	exceptions := make([]string, 0, len(excluded))
	for v := range excluded {
		exceptions = append(exceptions, v)
	}
	sort.Strings(exceptions)
	return nil, exceptions, true
}

// 检查小型有限集合成员，不依赖集合的外部排序状态。
// Check membership in a small finite set without relying on external sorting.
func stringMember(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// 判定一个请求维度是否满足包含和排除集合。
// Decide whether a request dimension satisfies inclusion and exclusion sets.
func conditionAllows(included, excluded []string, value string) bool {
	return (len(included) == 0 || stringMember(included, value)) && !stringMember(excluded, value)
}

// 从方法与显式媒体配置选择静态契约，并保留所选条件的来源。
// Select static contracts using the method and explicit media configuration, retaining condition provenance.
func linkConditionalTemplate(template Template, route Route) (spec.Operation, []Diagnostic, []Source, error) {
	op := copyJSON(template.Operation)
	diagnostics := append([]Diagnostic(nil), template.Diagnostics...)
	facts := append([]Source(nil), template.Facts...)
	if len(template.Variants) == 0 {
		return op, diagnostics, facts, nil
	}
	media, err := conditionSet(route.RequestMediaTypes, true)
	if err != nil {
		return op, diagnostics, facts, err
	}
	selected := map[int]bool{}
	if len(media) > 0 {
		when := RequestCondition{Methods: []string{route.Method}, MediaTypes: append([]string(nil), media...)}
		facts = append(facts, Source{Kind: "declared", Rule: "openapi.Route.RequestMediaTypes", When: &when})
	}
	if len(media) == 0 {
		for i, variant := range template.Variants {
			if conditionAllows(variant.When.Methods, variant.When.ExceptMethods, route.Method) {
				if len(variant.When.MediaTypes)+len(variant.When.ExceptMediaTypes) > 0 {
					return op, diagnostics, facts, fmt.Errorf("openapi.condition.media: %s %s 需要明确请求媒体类型", route.Method, route.Path)
				}
				selected[i] = true
			}
		}
	} else {
		for _, kind := range media {
			matched := false
			for i, variant := range template.Variants {
				when := variant.When
				if conditionAllows(when.Methods, when.ExceptMethods, route.Method) && conditionAllows(when.MediaTypes, when.ExceptMediaTypes, kind) {
					selected[i] = true
					matched = true
				}
			}
			if !matched {
				return op, diagnostics, facts, fmt.Errorf("openapi.condition.uncovered: %s %s 的媒体类型 %q 没有契约", route.Method, route.Path, kind)
			}
		}
	}
	if len(selected) == 0 {
		return op, diagnostics, facts, fmt.Errorf("openapi.condition.uncovered: %s %s 没有适用契约", route.Method, route.Path)
	}
	var parameters []byte
	var bodyRequired *bool
	for i, variant := range template.Variants {
		if selected[i] {
			// 跨媒体必填差异无法无损投射为统一的请求体布尔标志。
			// Media-dependent presence cannot be projected losslessly into one body-required boolean.
			required := variant.Operation.RequestBody != nil && variant.Operation.RequestBody.Value != nil && variant.Operation.RequestBody.Value.Required
			if bodyRequired != nil && *bodyRequired != required {
				return op, diagnostics, facts, fmt.Errorf("openapi.condition.ambiguous: 不同条件的请求体 required 不一致")
			}
			bodyRequired = &required
			parameterJSON, _ := json.Marshal(variant.Operation.Parameters)
			if parameters != nil && !bytes.Equal(parameters, parameterJSON) {
				return op, diagnostics, facts, fmt.Errorf("openapi.condition.ambiguous: 不同条件的参数契约不能无损合并")
			}
			parameters = parameterJSON
			if err := mergeConditionalOperation(&op, variant.Operation); err != nil {
				return op, diagnostics, facts, err
			}
			for _, diagnostic := range variant.Diagnostics {
				source := diagnostic.Source
				when := copyJSON(variant.When)
				source.When = &when
				diagnostic.Source = source
				diagnostics = append(diagnostics, diagnostic)
			}
			for _, source := range variant.Facts {
				when := copyJSON(variant.When)
				source.When = &when
				facts = append(facts, source)
			}
		}
	}
	return op, diagnostics, facts, nil
}

// 合并可共存的元数据字段，冲突必须诊断，不能按顺序覆盖。
// Merge compatible metadata fields and diagnose conflicts instead of overwriting by order.
func mergeConditionMetadata(target, source any, ignored ...string) error {
	leftRaw, err := json.Marshal(target)
	if err != nil {
		return err
	}
	rightRaw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	var left, right map[string]json.RawMessage
	if err = json.Unmarshal(leftRaw, &left); err != nil {
		return err
	}
	if err = json.Unmarshal(rightRaw, &right); err != nil {
		return err
	}
	for key, value := range right {
		if stringMember(ignored, key) {
			continue
		}
		if old, ok := left[key]; ok && !bytes.Equal(old, value) {
			return fmt.Errorf("openapi.condition.ambiguous: 条件分支的 %s 不一致", key)
		}
		left[key] = value
	}
	merged, err := json.Marshal(left)
	if err != nil {
		return err
	}
	return json.Unmarshal(merged, target)
}

// 合并条件操作的请求与响应表示，其余信息只接受兼容元数据。
// Merge conditional request and response representations while requiring compatible metadata elsewhere.
func mergeConditionalOperation(target *spec.Operation, source spec.Operation) error {
	if err := mergeConditionMetadata(target, source, "requestBody", "responses"); err != nil {
		return err
	}
	if source.RequestBody != nil {
		if target.RequestBody == nil {
			target.RequestBody = copyJSON(source.RequestBody)
		} else {
			if target.RequestBody.Value == nil || source.RequestBody.Value == nil {
				if !jsonSame(target.RequestBody, source.RequestBody) {
					return fmt.Errorf("openapi.condition.ambiguous: 请求体引用不一致")
				}
			} else {
				a, b := target.RequestBody.Value, source.RequestBody.Value
				if err := mergeConditionMetadata(a, b, "content"); err != nil {
					return err
				}
				if err := mergeConditionContent(&a.Content, b.Content); err != nil {
					return err
				}
			}
		}
	}
	if target.Responses == nil {
		target.Responses = map[string]spec.RefOr[spec.Response]{}
	}
	for status, incoming := range source.Responses {
		existing, found := target.Responses[status]
		if !found {
			target.Responses[status] = copyJSON(incoming)
			continue
		}
		if existing.Value == nil || incoming.Value == nil {
			if !jsonSame(existing, incoming) {
				return fmt.Errorf("openapi.condition.ambiguous: 响应引用不一致")
			}
			continue
		}
		a, b := existing.Value, incoming.Value
		if err := mergeConditionMetadata(a, b, "content", "headers"); err != nil {
			return err
		}
		if err := mergeConditionContent(&a.Content, b.Content); err != nil {
			return err
		}
		if a.Headers == nil && len(b.Headers) > 0 {
			a.Headers = map[string]spec.RefOr[spec.Header]{}
		}
		for name, header := range b.Headers {
			old, present := a.Headers[name]
			if !present {
				a.Headers[name] = copyJSON(header)
				continue
			}
			if old.Value == nil || header.Value == nil {
				if !jsonSame(old, header) {
					return fmt.Errorf("openapi.condition.ambiguous: 响应头引用不一致")
				}
				continue
			}
			if err := mergeConditionMetadata(old.Value, header.Value, "schema"); err != nil {
				return err
			}
			old.Value.Schema = conditionalSchemaUnion(old.Value.Schema, header.Value.Schema)
		}
	}
	return nil
}

// 比较 JSON 语义的确定性编码，避免指针身份影响相等判断。
// Compare deterministic JSON encodings without relying on pointer identity.
func jsonSame(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && bytes.Equal(left, right)
}

// 合并不同媒体类型，重叠的 Schema 使用允许重叠的 anyOf。
// Merge distinct media types and use anyOf for overlapping schema alternatives.
func mergeConditionContent(target *map[string]spec.RefOr[spec.MediaType], source map[string]spec.RefOr[spec.MediaType]) error {
	if *target == nil && len(source) > 0 {
		*target = map[string]spec.RefOr[spec.MediaType]{}
	}
	for kind, media := range source {
		old, ok := (*target)[kind]
		if !ok {
			(*target)[kind] = copyJSON(media)
			continue
		}
		if old.Value == nil || media.Value == nil {
			if !jsonSame(old, media) {
				return fmt.Errorf("openapi.condition.ambiguous: 媒体类型引用不一致")
			}
			continue
		}
		// 分别约束完整正文和流条目的条件不能退化为无约束媒体对象。
		// Conditions constraining complete bodies versus stream items must not degrade into unconstrained media objects.
		a, b := old.Value, media.Value
		if (a.ItemSchema != nil && a.Schema == nil && b.Schema != nil && b.ItemSchema == nil) ||
			(b.ItemSchema != nil && b.Schema == nil && a.Schema != nil && a.ItemSchema == nil) {
			return fmt.Errorf("openapi.condition.ambiguous: 完整正文与逐项响应的条件分帧方式不一致")
		}
		if err := mergeConditionMetadata(old.Value, media.Value, "schema", "itemSchema", "encoding"); err != nil {
			return err
		}
		// 编码按字段合并；不同字段可共存，同字段必须保持相同表示。
		// Merge encodings by field; distinct fields can coexist, while shared fields must keep the same representation.
		keys := make([]string, 0, len(media.Value.Encoding))
		for key := range media.Value.Encoding {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			encoding := media.Value.Encoding[key]
			if previous, exists := old.Value.Encoding[key]; exists && !jsonSame(previous, encoding) {
				return fmt.Errorf("openapi.condition.ambiguous: 请求字段 %s 的 encoding 不一致", key)
			}
			if old.Value.Encoding == nil {
				old.Value.Encoding = map[string]spec.Encoding{}
			}
			old.Value.Encoding[key] = copyJSON(encoding)
		}
		old.Value.Schema = conditionalSchemaUnion(old.Value.Schema, media.Value.Schema)
		if old.Value.ItemSchema != nil || media.Value.ItemSchema != nil {
			old.Value.ItemSchema = conditionalSchemaUnion(old.Value.ItemSchema, media.Value.ItemSchema)
		}
	}
	return nil
}

// 将条件 Schema 合并为独立副本；没有约束的分支不能被更严格分支收窄。
// Merge conditional schemas into detached copies without narrowing an unconstrained alternative.
func conditionalSchemaUnion(a, b *spec.Schema) *spec.Schema {
	if a == nil || b == nil {
		return nil
	}
	if jsonSame(a, b) {
		return copyJSON(a)
	}

	return &spec.Schema{SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{copyJSON(a), copyJSON(b)}}}
}
