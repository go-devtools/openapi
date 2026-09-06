package compiler

import (
	"encoding/json"
	"fmt"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Collect request constraints that hold together on one execution path, with fields and codecs supplied by frontends.
// 收集同一执行路径中共同成立的请求约束，框架负责说明字段和编码来源。
type requestMedia struct {
	schemas  []*spec.Schema
	fields   *spec.Schema
	encoding map[string]spec.Encoding
}

// Prefer explicit wire representations over type projection and detach frontend-owned schemas.
// 明确网络表示优先于类型投影，所有返回结果与前端对象隔离。
func (p *Project) requestSchema(effect Effect, components map[string]*spec.Schema, mappers []TypeMapper) (*spec.Schema, error) {
	if effect.WireSchema != nil {
		return copyWireSchema(effect.WireSchema)
	}
	return p.valueSchema(effect.Payload, Input, effect.MediaType, effect.Codec, mappers, components)
}

// Merge individual field reads and whole-object reads by intersection within a path rather than alternatives.
// 合并逐字段读取和完整对象读取；同一路径使用交集而不是备选。
func (p *Project) requestPath(path flow, components map[string]*spec.Schema, mappers []TypeMapper) (*spec.RequestBody, openapi.Source, error) {
	groups := map[string]*requestMedia{}
	required := false
	var source openapi.Source
	for _, effect := range path.effects {
		if effect.Kind != RequestBody && effect.Kind != RequestField {
			continue
		}
		source = effect.Source
		if effect.MediaType == "" {
			return nil, source, fmt.Errorf("request media type is unresolved")
		}
		schema, err := p.requestSchema(effect, components, mappers)
		if err != nil {
			return nil, source, err
		}
		if effect.AlternativeLocations {
			if effect.Required {
				return nil, source, fmt.Errorf("required relationships across input locations need a centralized contract")
			}
			if err := alternativeLocationRequirements(schema, components); err != nil {
				return nil, source, err
			}
		}
		group := groups[effect.MediaType]
		if group == nil {
			group = &requestMedia{encoding: map[string]spec.Encoding{}}
			groups[effect.MediaType] = group
		}
		if effect.Kind == RequestBody {
			required = required || effect.Required
			group.schemas = appendUniqueSchema(group.schemas, schema)
			continue
		}
		if effect.Name == "" {
			return nil, source, fmt.Errorf("request field name is not an explicit nonempty constant")
		}
		if group.fields == nil {
			group.fields = spec.Typed("object")
			group.fields.Properties = map[string]*spec.Schema{}
		}
		if previous := group.fields.Properties[effect.Name]; previous != nil && !sameRequestJSON(previous, schema) {
			return nil, source, fmt.Errorf("request field %s has inconsistent wire representation", effect.Name)
		}
		group.fields.Properties[effect.Name] = schema
		if effect.Required {
			names := map[string]bool{}
			for _, name := range group.fields.Required.Value {
				names[name] = true
			}
			names[effect.Name] = true
			group.fields.Required = spec.Set(sortedKeys(names))
		}
		if effect.Encoding != nil {
			var encoding spec.Encoding
			raw, err := json.Marshal(effect.Encoding)
			if err != nil {
				return nil, source, err
			}
			if err = json.Unmarshal(raw, &encoding); err != nil {
				return nil, source, err
			}
			if old, ok := group.encoding[effect.Name]; ok && !sameRequestJSON(old, encoding) {
				return nil, source, fmt.Errorf("request field %s has inconsistent encoding", effect.Name)
			}
			group.encoding[effect.Name] = encoding
		}
	}
	if len(groups) == 0 {
		return nil, source, nil
	}
	body := &spec.RequestBody{Required: required, Content: map[string]spec.RefOr[spec.MediaType]{}}
	for _, media := range sortedKeys(groups) {
		group := groups[media]
		if group.fields != nil {
			group.schemas = appendUniqueSchema(group.schemas, group.fields)
		}
		schema := group.schemas[0]
		if len(group.schemas) > 1 {
			schema = &spec.Schema{SchemaObject: &spec.SchemaObject{AllOf: group.schemas}}
		}
		body.Content[media] = spec.Inline(spec.MediaType{Schema: schema, Encoding: group.encoding})
	}
	return body, source, nil
}

// Deduplicate repeated constraints within a path without introducing extra composition layers.
// 同一路径重复观察相同约束不增加组合层级。
func appendUniqueSchema(schemas []*spec.Schema, schema *spec.Schema) []*spec.Schema {
	for _, existing := range schemas {
		if sameRequestJSON(existing, schema) {
			return schemas
		}
	}
	return append(schemas, schema)
}

// Compare serialized specification values independently of pointers and map traversal order.
// 比较已序列化的规范值，不依赖指针或 map 遍历顺序。
func sameRequestJSON(left, right any) bool {
	a, ea := json.Marshal(left)
	b, eb := json.Marshal(right)
	return ea == nil && eb == nil && string(a) == string(b)
}

// Combine different paths as alternatives, requiring a body only when every path requires one.
// 不同执行路径使用备选；只有所有路径都要求请求体时才标为必填。
func (p *Project) mergeRequestPaths(operation *spec.Operation, diagnostics *[]openapi.Diagnostic, paths []flow, components map[string]*spec.Schema, mappers []TypeMapper) {
	required := len(paths) > 0
	var merged *spec.RequestBody
	for _, path := range paths {
		body, source, err := p.requestPath(path, components, mappers)
		if err != nil {
			if detailed := projectionDiagnostics(err, source); len(detailed) > 0 {
				*diagnostics = append(*diagnostics, detailed...)
				continue
			}
			*diagnostics = append(*diagnostics, openapi.Diagnostic{Code: "openapi.effect.unresolved", Severity: openapi.Error, Message: err.Error(), Fix: "Provide consistent centralized request-field or encoding rules", Source: source})
			continue
		}
		required = required && body != nil && body.Required
		if body == nil {
			continue
		}
		if merged == nil {
			merged = body
			continue
		}
		for _, name := range sortedKeys(body.Content) {
			incoming := body.Content[name].Value
			old := merged.Content[name]
			if old.Value == nil {
				merged.Content[name] = body.Content[name]
				continue
			}
			for _, field := range sortedKeys(incoming.Encoding) {
				encoding := incoming.Encoding[field]
				if previous, ok := old.Value.Encoding[field]; ok && !sameRequestJSON(previous, encoding) {
					*diagnostics = append(*diagnostics, openapi.Diagnostic{Code: "openapi.effect.unresolved", Severity: openapi.Error, Message: "request field alternatives have inconsistent encodings: " + field, Fix: "Provide consistent centralized encoding rules", Source: source})
				} else {
					if old.Value.Encoding == nil {
						old.Value.Encoding = map[string]spec.Encoding{}
					}
					old.Value.Encoding[field] = encoding
				}
			}
			old.Value.Schema = union(old.Value.Schema, incoming.Schema)
		}
	}
	if merged != nil {
		merged.Required = required
		body := spec.Inline(*merged)
		operation.RequestBody = &body
	}
}
