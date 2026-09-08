package compiler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-devtools/openapi/spec"
)

// Expand an explicit object projection into named parameters, reusing field annotations and component references.
func (p *Project) mergeParameterObject(op *spec.Operation, effect Effect, components map[string]*spec.Schema, mappers []TypeMapper) error {
	if effect.In != "query" && effect.In != "path" && effect.In != "header" && effect.In != "cookie" {
		return fmt.Errorf("invalid parameter object location: %s", effect.In)
	}
	schema, err := p.valueSchema(effect.Payload, Input, effect.MediaType, effect.Codec, mappers, components, p.effectUse(effect, Input))
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
			return fmt.Errorf("parameter object root reference cannot be expanded unambiguously: %s", ref)
		}
		seen[ref] = true
		schema = components[strings.TrimPrefix(ref, "#/components/schemas/")]
	}
	if schema == nil || schema.SchemaObject == nil || len(schema.Type) != 1 || schema.Type[0] != "object" || len(schema.AnyOf) > 0 || len(schema.AllOf) > 0 || len(schema.OneOf) > 0 || schema.AdditionalProperties != nil {
		return fmt.Errorf("parameter binding requires an object projection with explicit fields")
	}
	if err := parameterObjectKeywords(schema, false); err != nil {
		return err
	}
	if effect.AlternativeLocations && len(schema.Required.Value) > 0 {
		return fmt.Errorf("required relationships across input locations need a centralized contract; requiring every location is incorrect")
	}
	required := map[string]bool{}
	for _, name := range schema.Required.Value {
		if schema.Properties[name] == nil {
			return fmt.Errorf("parameter object requires an explicit field that does not exist: %s", name)
		}
		required[name] = true
	}
	for _, name := range sortedKeys(schema.Properties) {
		field, err := copyWireSchema(schema.Properties[name])
		if err != nil {
			return err
		}
		parameter := spec.Parameter{Name: name, In: effect.In, Required: spec.Set(required[name] || effect.In == "path"), Schema: field, Style: effect.Style, Explode: effect.Explode}
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
				return fmt.Errorf("parameter %s/%s has inconsistent projection or serialization", effect.In, name)
			}
			matched = true
		}
		if !matched {
			op.Parameters = append(op.Parameters, spec.Inline(parameter))
		}
	}
	return nil
}

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
		return fmt.Errorf("parameter object %s cannot be preserved without loss during expansion; register a centralized parameter rule", key)
	}
	return nil
}

// Check presence requirements across alternative locations without disguising them as local property constraints.
func alternativeLocationRequirements(schema *spec.Schema, components map[string]*spec.Schema) error {
	seen := map[string]bool{}
	for schema != nil && schema.SchemaObject != nil && schema.Ref != "" {
		ref := schema.Ref
		if seen[ref] || !strings.HasPrefix(ref, "#/components/schemas/") {
			return fmt.Errorf("root reference across input locations cannot be resolved unambiguously")
		}
		seen[ref] = true
		schema = components[strings.TrimPrefix(ref, "#/components/schemas/")]
	}
	if schema == nil || schema.SchemaObject == nil || len(schema.Required.Value) > 0 {
		return fmt.Errorf("required relationships across input locations need a centralized contract")
	}
	return nil
}

// Preserve explicit single-field wire types and serialization without silently replacing conflicting observations.
func (p *Project) mergeParameterRead(operation *spec.Operation, effect Effect, components map[string]*spec.Schema, mappers []TypeMapper) error {
	if effect.Name == "" {
		return fmt.Errorf("parameter name is not an evaluable constant")
	}
	if effect.MediaType == "" {
		effect.MediaType = "application/json"
	}
	schema, err := p.requestSchema(effect, components, mappers)
	if err != nil {
		return err
	}
	parameter := spec.Parameter{Name: effect.Name, In: effect.In, Required: spec.Set(effect.Required || effect.In == "path"), Schema: schema, Style: effect.Style, Explode: effect.Explode}
	for _, existing := range operation.Parameters {
		if existing.Value == nil || existing.Value.Name != effect.Name || existing.Value.In != effect.In {
			continue
		}
		if !sameRequestJSON(existing.Value, parameter) {
			return fmt.Errorf("parameter %s/%s has inconsistent projection or serialization", effect.In, effect.Name)
		}
		return nil
	}
	operation.Parameters = append(operation.Parameters, spec.Inline(parameter))
	return nil
}
