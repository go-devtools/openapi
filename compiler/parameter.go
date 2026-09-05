package compiler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openapi-golang/openapi/spec"
)

// 将明确对象投影展开为命名参数，复用共享字段注释与组件引用。
// Expand an explicit object projection into named parameters, reusing field annotations and component references.
func (p *Project) mergeParameterObject(op *spec.Operation, effect Effect, components map[string]*spec.Schema, mappers []TypeMapper) error {
	if effect.In != "query" && effect.In != "path" && effect.In != "header" && effect.In != "cookie" {
		return fmt.Errorf("参数对象的位置无效：%s", effect.In)
	}
	schema, err := p.valueSchema(effect.Payload, Input, effect.MediaType, effect.Codec, mappers, components)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for schema != nil && schema.SchemaObject != nil && schema.Ref != "" {
		if err := parameterObjectKeywords(schema, true); err != nil {
			return err
		}
		ref := schema.Ref
		if seen[ref] || !strings.HasPrefix(ref, "#/components/schemas/") {
			return fmt.Errorf("参数对象根引用无法明确展开：%s", ref)
		}
		seen[ref] = true
		schema = components[strings.TrimPrefix(ref, "#/components/schemas/")]
	}
	if schema == nil || schema.SchemaObject == nil || len(schema.Type) != 1 || schema.Type[0] != "object" || len(schema.AnyOf) > 0 || len(schema.AllOf) > 0 || len(schema.OneOf) > 0 || schema.AdditionalProperties != nil {
		return fmt.Errorf("参数绑定需要具有明确字段的对象投影")
	}
	if err := parameterObjectKeywords(schema, false); err != nil {
		return err
	}
	required := map[string]bool{}
	for _, name := range schema.Required.Value {
		if schema.Properties[name] == nil {
			return fmt.Errorf("参数对象要求不存在的明确字段：%s", name)
		}
		required[name] = true
	}
	for _, name := range sortedKeys(schema.Properties) {
		field, err := copyWireSchema(schema.Properties[name])
		if err != nil {
			return err
		}
		parameter := spec.Parameter{Name: name, In: effect.In, Required: required[name] || effect.In == "path", Schema: field, Style: effect.Style, Explode: effect.Explode}
		if field.SchemaObject != nil {
			parameter.Description = field.Description
		}
		matched := false
		for _, existing := range op.Parameters {
			if existing.Value == nil || existing.Value.Name != name || existing.Value.In != effect.In {
				continue
			}
			left, _ := json.Marshal(existing.Value)
			right, _ := json.Marshal(parameter)
			if string(left) != string(right) {
				return fmt.Errorf("同名参数 %s/%s 的投影或序列化不一致", effect.In, name)
			}
			matched = true
		}
		if !matched {
			op.Parameters = append(op.Parameters, spec.Inline(parameter))
		}
	}
	return nil
}

// 参数展开只能保留逐字段约束，整体约束与资源作用域必须明确拒绝。
// Parameter expansion can preserve only per-field constraints; reject whole-object constraints and resource scopes explicitly.
func parameterObjectKeywords(schema *spec.Schema, reference bool) error {
	raw, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	for _, key := range sortedKeys(object) {
		switch key {
		case "title", "description", "$comment", "examples", "example":
			continue
		case "$ref":
			if reference {
				continue
			}
		case "type", "properties", "required":
			if !reference {
				continue
			}
		}
		return fmt.Errorf("参数对象的 %s 不能在展开时无损保留，需要集中参数规则", key)
	}
	return nil
}
