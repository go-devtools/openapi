package compiler

import (
	"encoding/json"
	"fmt"
	"go/types"
	"mime"
	"strconv"
	"strings"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/internal/comment"
	"github.com/go-devtools/openapi/spec"
)

// Preserve resolved annotation data independently of inferred control-flow effects.
type contractDeclaration struct {
	kind, status, media string
	typ                 types.Type
	required            spec.Optional[bool]
	source              openapi.Source
}

// Resolve function-local declarations once; invalid candidates remain isolated until route selection.
func (p *Project) declarations(fn Function, diagnostics *[]openapi.Diagnostic) []contractDeclaration {
	var declarations []contractDeclaration
	positions := p.directivePositions(fn.Declaration.Doc)
	for i, directive := range p.comments[fn.Object].Directives {
		if directive.Kind == "" {
			continue
		}
		source := fn.Source
		if i < len(positions) {
			source = p.Source(positions[i])
		}
		source.Kind, source.Rule, source.Symbol = "declared", "openapi.comment."+directive.Kind, fn.Symbol
		declaration, err := parseContractDeclaration(directive)
		code := "openapi.declaration.invalid"
		if err == nil && directive.Values["type"] != nil {
			var name string
			_ = json.Unmarshal(directive.Values["type"], &name)
			declaration.typ, err = p.TypeIn(fn.Package.Path, name)
			code = "openapi.declaration.type"
		}
		if err != nil {
			*diagnostics = append(*diagnostics, openapi.Diagnostic{Code: code, Severity: openapi.Error, Message: err.Error(), Fix: "Use an accurate declaration and a complete type from the explicitly loaded packages", Source: source})
			continue
		}
		for _, previous := range declarations {
			if previous.kind == declaration.kind && previous.media == declaration.media && previous.status == declaration.status && previous.required.Present && declaration.required.Present && previous.required.Value != declaration.required.Value {
				*diagnostics = append(*diagnostics, openapi.Diagnostic{Code: "openapi.declaration.conflict", Severity: openapi.Error, Message: "repeated request declarations disagree on required body presence", Fix: "Declare one consistent request-body presence constraint", Source: source, Facts: []openapi.Source{previous.source}})
			}
		}
		declaration.source = source
		declarations = append(declarations, declaration)
	}
	return declarations
}

// Require explicit payload media and type; only response declarations may omit both for a bodyless result.
func parseContractDeclaration(d comment.Directive) (contractDeclaration, error) {
	result := contractDeclaration{kind: d.Kind}
	for _, key := range sortedKeys(d.Values) {
		raw := d.Values[key]
		switch key {
		case "type":
			var name string
			if err := json.Unmarshal(raw, &name); err != nil || strings.TrimSpace(name) == "" {
				return result, fmt.Errorf("declaration type must be a nonempty Go type expression")
			}
		case "mediaType":
			if err := json.Unmarshal(raw, &result.media); err != nil {
				return result, fmt.Errorf("declaration mediaType must be a string")
			}
			media, params, err := mime.ParseMediaType(result.media)
			if err != nil || len(params) != 0 || !strings.Contains(media, "/") || strings.Contains(media, "*") || media != result.media {
				return result, fmt.Errorf("declaration mediaType must be a concrete lowercase media type without parameters")
			}
		case "required":
			if d.Kind != "request" || string(raw) != "true" && string(raw) != "false" {
				return result, fmt.Errorf("only request declarations accept a boolean required value")
			}
			result.required = spec.Set(string(raw) == "true")
		case "status":
			if d.Kind != "response" {
				return result, fmt.Errorf("only response declarations accept status")
			}
			if json.Unmarshal(raw, &result.status) != nil {
				result.status = string(raw)
			}
			code, err := strconv.Atoi(result.status)
			if result.status != "default" && (err != nil || code < 200 || code > 599 || strconv.Itoa(code) != result.status) {
				return result, fmt.Errorf("declaration status must be an integer from 200 to 599 or the string default")
			}
		default:
			return result, fmt.Errorf("%s declarations do not accept %s", d.Kind, key)
		}
	}
	typed := d.Values["type"] != nil
	if typed != (result.media != "") || d.Kind == "request" && !typed {
		return result, fmt.Errorf("a payload declaration requires both mediaType and type")
	}
	if d.Kind == "response" && result.status == "" {
		return result, fmt.Errorf("a response declaration requires an explicit status")
	}
	if typed && bodylessStatus(result.status) {
		return result, fmt.Errorf("response status %s cannot carry a body", result.status)
	}
	return result, nil
}

// Supplement contracts after deriving all paths; declarations never remove analysis diagnostics or observed responses.
func (p *Project) mergeDeclarations(template *openapi.Template, declarations []contractDeclaration, paths []flow, components map[string]*spec.Schema, mappers []TypeMapper) {
	if len(declarations) == 0 {
		return
	}
	if len(template.Variants) == 0 {
		p.mergeOperationDeclarations(&template.Operation, &template.Diagnostics, &template.Facts, declarations, paths, components, mappers)
		return
	}
	for i := range template.Variants {
		variant := &template.Variants[i]
		var selected []flow
		for _, path := range paths {
			if sameRequestJSON(path.when, variant.When) {
				selected = append(selected, path)
			}
		}
		scoped := append([]contractDeclaration(nil), declarations...)
		for j := range scoped {
			when := variant.When
			scoped[j].source.When = &when
		}
		p.mergeOperationDeclarations(&variant.Operation, &variant.Diagnostics, &variant.Facts, scoped, selected, components, mappers)
	}
}

// Reuse a known matching wire codec, diagnosing ambiguous codecs rather than assuming JSON for foreign representations.
func declarationCodec(d contractDeclaration, paths []flow) (WireCodec, error) {
	found := false
	var codec WireCodec
	name := ""
	for _, path := range paths {
		for _, effect := range path.effects {
			matches := d.kind == "request" && effect.Kind == RequestBody || d.kind == "response" && effect.Kind == ResponseBody && effect.Status == d.status
			if !matches || effect.MediaType != d.media {
				continue
			}
			incoming := ""
			if effect.Codec != nil {
				incoming = effect.Codec.Name()
			}
			if found && name != incoming {
				return nil, fmt.Errorf("declaration matches different wire codecs; provide one centralized contract rule")
			}
			codec, name, found = effect.Codec, incoming, true
		}
	}
	return codec, nil
}

// Compare matching wire schemas conservatively; different shapes require an explicit centralized rule rather than silent unions.
func (p *Project) mergeOperationDeclarations(op *spec.Operation, diagnostics *[]openapi.Diagnostic, facts *[]openapi.Source, declarations []contractDeclaration, paths []flow, components map[string]*spec.Schema, mappers []TypeMapper) {
	for _, d := range declarations {
		*facts = append(*facts, d.source)
		var schema *spec.Schema
		codec, err := declarationCodec(d, paths)
		if err == nil && d.typ != nil {
			direction := Output
			if d.kind == "request" {
				direction = Input
			}
			schema, err = p.valueSchema(Value{Type: d.typ}, direction, d.media, codec, mappers, components, p.effectUse(Effect{Kind: EffectKind(d.kind + "Body"), Status: d.status, MediaType: d.media, Codec: codec, Payload: Value{Type: d.typ}, Source: d.source}, direction))
		}
		code := "openapi.declaration.schema"
		if err == nil {
			code = "openapi.declaration.conflict"
			err = mergeDeclaredContract(op, d, schema)
		}
		if err != nil {
			if detailed := projectionDiagnostics(err, d.source); len(detailed) > 0 {
				*diagnostics = append(*diagnostics, detailed...)
				continue
			}
			*diagnostics = append(*diagnostics, openapi.Diagnostic{Code: code, Severity: openapi.Error, Message: err.Error(), Source: d.source, Facts: append([]openapi.Source(nil), (*facts)...), Fix: "Keep declarations consistent with observed wire facts; use a centralized frontend or codec rule for unsupported behavior"})
		}
	}
}

// Add absent alternatives and semantic presence constraints while preserving every already observed contract.
func mergeDeclaredContract(op *spec.Operation, d contractDeclaration, schema *spec.Schema) error {
	if d.kind == "request" {
		if op.RequestBody == nil {
			body := spec.Inline(spec.RequestBody{Content: map[string]spec.RefOr[spec.MediaType]{}})
			op.RequestBody = &body
		}
		body := op.RequestBody.Value
		if body == nil {
			return fmt.Errorf("request declaration cannot replace a referenced request body")
		}
		if previous := body.Content[d.media].Value; previous != nil && (previous.ItemSchema != nil || !sameRequestJSON(previous.Schema, schema)) {
			return fmt.Errorf("declared request type conflicts with the observed schema for %s", d.media)
		}
		if d.required.Present && !d.required.Value && body.Required.Value {
			return fmt.Errorf("declared optional body conflicts with a required request body")
		}
		if body.Content == nil {
			body.Content = map[string]spec.RefOr[spec.MediaType]{}
		}
		if _, exists := body.Content[d.media]; !exists {
			body.Content[d.media] = spec.Inline(spec.MediaType{Schema: schema})
		}
		if d.required.Present {
			body.Required = d.required
		}
		return nil
	}
	if op.Responses == nil {
		op.Responses = map[string]spec.RefOr[spec.Response]{}
	}
	response, exists := op.Responses[d.status]
	if exists {
		if response.Value == nil {
			return fmt.Errorf("response declaration cannot replace a referenced response")
		}
		if schema == nil && len(response.Value.Content) > 0 || schema != nil && len(response.Value.Content) == 0 {
			return fmt.Errorf("declared body presence conflicts with response %s", d.status)
		}
		if previous := response.Value.Content[d.media].Value; previous != nil && (previous.ItemSchema != nil || !sameRequestJSON(previous.Schema, schema)) {
			return fmt.Errorf("declared response type conflicts with the observed schema for %s %s", d.status, d.media)
		}
	} else {
		response = spec.Inline(spec.Response{Description: "Response " + d.status})
	}
	if schema != nil {
		if response.Value.Content == nil {
			response.Value.Content = map[string]spec.RefOr[spec.MediaType]{}
		}
		if _, exists := response.Value.Content[d.media]; !exists {
			response.Value.Content[d.media] = spec.Inline(spec.MediaType{Schema: schema})
		}
	}
	op.Responses[d.status] = response
	return nil
}
