package compiler

import (
	"encoding/json"
	"fmt"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/spec"
)

// Collect request constraints that hold together on one execution path, with fields and codecs supplied by frontends.
type requestMedia struct {
	schemas  []*spec.Schema
	fields   *spec.Schema
	encoding map[string]spec.Encoding
}

// Prefer explicit wire representations over type projection and detach frontend-owned schemas.
func (p *Project) requestSchema(effect Effect, components map[string]*spec.Schema, mappers []TypeMapper) (*spec.Schema, error) {
	if effect.WireSchema != nil {
		schema, err := copyWireSchema(effect.WireSchema)
		p.captureProjection(p.effectUse(effect, Input), &Projection{Root: schema, Audit: []string{"The frontend supplied a wire Schema; no Go field projection was performed."}}, err)
		return schema, err
	}
	return p.valueSchema(effect.Payload, Input, effect.MediaType, effect.Codec, mappers, components, p.effectUse(effect, Input))
}

// Merge individual field reads and whole-object reads by intersection within a path rather than alternatives.
func (p *Project) requestPath(path flow, components map[string]*spec.Schema, mappers []TypeMapper) (*spec.RequestBody, openapi.Source, error) {
	defer p.explanationPath(path.when)()
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
	body := &spec.RequestBody{Required: spec.Set(required), Content: map[string]spec.RefOr[spec.MediaType]{}}
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
func appendUniqueSchema(schemas []*spec.Schema, schema *spec.Schema) []*spec.Schema {
	for _, existing := range schemas {
		if sameRequestJSON(existing, schema) {
			return schemas
		}
	}
	return append(schemas, schema)
}

// Compare serialized specification values independently of pointers and map traversal order.
func sameRequestJSON(left, right any) bool {
	a, ea := json.Marshal(left)
	b, eb := json.Marshal(right)
	return ea == nil && eb == nil && string(a) == string(b)
}

// Combine different paths as alternatives, requiring a body only when every path requires one.
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
		required = required && body != nil && body.Required.Value
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
		merged.Required = spec.Set(required || inferredBodyRequired(paths))
		if !merged.Required.Value && rejectedBodyPaths(paths) {
			// No accepted path supplies a presence policy; a later explicit declaration may still set one.
			merged.Required = spec.Optional[bool]{}
		}
		body := spec.Inline(*merged)
		operation.RequestBody = &body
	}
}
