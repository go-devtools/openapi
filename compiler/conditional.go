package compiler

import (
	"encoding/json"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Merge complete analysis paths with identical request conditions while isolating diagnostics for other conditions.
func (p *Project) mergeConditionalPaths(template *openapi.Template, paths []flow, components map[string]*spec.Schema, mappers []TypeMapper) {
	conditional := false
	for _, path := range paths {
		conditional = conditional || !path.when.Unconditional()
	}
	if !conditional {
		for _, path := range paths {
			p.mergePath(&template.Operation, &template.Diagnostics, &template.Facts, path, components, mappers)
		}
		p.mergeRequestPaths(&template.Operation, &template.Diagnostics, paths, components, mappers)
		return
	}
	groupPaths := map[string][]flow{}
	groups := map[string]*openapi.OperationVariant{}
	for _, path := range paths {
		key, _ := json.Marshal(path.when)
		groupPaths[string(key)] = append(groupPaths[string(key)], path)
		variant := groups[string(key)]
		if variant == nil {
			variant = &openapi.OperationVariant{When: path.when, Operation: spec.Operation{Responses: map[string]spec.RefOr[spec.Response]{}}}
			groups[string(key)] = variant
		}
		p.mergePath(&variant.Operation, &variant.Diagnostics, &variant.Facts, path, components, mappers)
	}
	for _, key := range sortedKeys(groups) {
		p.mergeRequestPaths(&groups[key].Operation, &groups[key].Diagnostics, groupPaths[key], components, mappers)
		template.Variants = append(template.Variants, *groups[key])
	}
}

// Reuse shared projection and provenance rules without putting callbacks or Go types into runtime paths.
func (p *Project) mergePath(operation *spec.Operation, diagnostics *[]openapi.Diagnostic, facts *[]openapi.Source, path flow, components map[string]*spec.Schema, mappers []TypeMapper) {
	*diagnostics = append(*diagnostics, path.diagnostics...)
	for _, effect := range path.effects {
		*facts = append(*facts, effect.Source)
		for _, name := range sortedKeys(effect.Headers) {
			*facts = append(*facts, effect.Headers[name].Source)
		}
		if effect.Kind == RequestBody || effect.Kind == RequestField {
			continue
		}
		if err := p.mergeEffect(operation, effect, components, mappers); err != nil {
			if detailed := projectionDiagnostics(err, effect.Source); len(detailed) > 0 {
				*diagnostics = append(*diagnostics, detailed...)
				continue
			}
			*diagnostics = append(*diagnostics, openapi.Diagnostic{Code: "openapi.effect.unresolved", Severity: openapi.Error, Message: err.Error(), Fix: "Register a centralized frontend rule or TypeMapper", Source: effect.Source})
		}
	}
}
