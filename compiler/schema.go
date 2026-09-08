package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/internal/comment"
	"github.com/go-devtools/openapi/internal/validate"
	"github.com/go-devtools/openapi/spec"
)

// Identify input and output wire projections.
type Direction string

// Compute input constraints and output presence independently.
const (
	Input  Direction = "request"
	Output Direction = "response"
)

// Map custom types centrally without executing user codecs.
type TypeMapper func(ProjectionRequest) (*spec.Schema, bool, error)

// Describe a wire field and its actual encoding properties.
type WireField struct {
	Name          string
	Field         *types.Var
	OmitEmpty     bool
	StringEncoded bool
	Optional      bool
}

// Let adapters select fields without introducing framework tags into the core.
type WireCodec interface {
	Name() string
	Fields(*types.Struct) ([]WireField, error)
}

// Codecs may explicitly override type projection; recursive callbacks share the current budget and reference cache.
type WireTypeCodec interface {
	ProjectType(ProjectionRequest, func(types.Type) (*spec.Schema, error)) (*spec.Schema, bool, error)
}

// Include type, direction, media type, and codec in projection identity.
type ProjectionRequest struct {
	// Collect source origins during this projection without rerunning rules.
	Explain   bool
	Type      types.Type
	Direction Direction
	MediaType string
	Codec     WireCodec
	Mappers   []TypeMapper
	MaxTypes  int
}

// Return a root Schema and its complete component closure.
type Projection struct {
	// Optional evidence describes the selected wire projection, not runtime enforcement.
	Origins    []SchemaOrigin
	Rules      []openapi.Source
	Root       *spec.Schema
	Components map[string]*spec.Schema
	Audit      []string
}

// Keep a private recursion cache for one projection.
type projector struct {
	origins     []SchemaOrigin
	rules       []openapi.Source
	project     *Project
	request     ProjectionRequest
	components  map[string]*spec.Schema
	seen        map[string]string
	count       int
	annotations []annotationCheck
}

// Project actual types and indexed comments without invoking their methods.
func (p *Project) Schema(request ProjectionRequest) (*Projection, error) {
	if request.Type == nil {
		return nil, fmt.Errorf("openapi.schema.type: actual Go type is required")
	}
	if request.Direction != Input && request.Direction != Output {
		return nil, fmt.Errorf("openapi.schema.direction: request or response is required")
	}
	if request.MediaType == "" {
		request.MediaType = "application/json"
	}
	if request.Codec == nil && request.MediaType != "application/json" {
		return nil, fmt.Errorf("openapi.codec.unknown: media type %s requires a centralized WireCodec", request.MediaType)
	}
	if request.MaxTypes == 0 {
		request.MaxTypes = 4096
	}
	pr := projector{project: p, request: request, components: map[string]*spec.Schema{}, seen: map[string]string{}}
	root, err := pr.projectType(request.Type)
	if err != nil {
		return nil, err
	}
	if err = pr.checkAnnotations(); err != nil {
		return nil, err
	}
	audit := []string{"Request Schema describes the declared contract; it does not prove that business code enforces every constraint."}
	if request.Codec == nil {
		audit = append(audit, "The full set of standard JSON decoder allowances for null, case folding, and fixed arrays is not modeled.")
	} else {
		audit = append(audit, fmt.Sprintf("Use explicit codec %s; the core does not execute actual decoding methods.", request.Codec.Name()))
	}
	if request.Explain {
		codec := "std-json"
		if request.Codec != nil {
			codec = request.Codec.Name()
		}
		pr.recordRule(request.Type, codec, "derived")
	}
	return &Projection{Root: root, Components: pr.components, Audit: audit, Origins: pr.origins, Rules: pr.rules}, nil
}

// Configure portable schema bases, offline dependencies, and bounded export with read-only inputs.
type StandaloneOptions struct {
	// Set the root schema's absolute retrieval URI for resolving relative resource identities.
	BaseURI string
	// Read only explicitly supplied JSON Schema documents without loading URIs from networks or files.
	Resources map[string][]byte
	// Use this dialect only when the root has none; default to JSON Schema 2020-12.
	Dialect string
	// Zero selects the core offline checker's input, resource, reference, and index budgets.
	MaxBytes, MaxResources, MaxReferences, MaxIndexBytes int
	// Zero selects a sixteen-MiB normalized output budget.
	MaxNormalizedBytes int
}

// Export a standalone schema with default budgets, using JSON Schema 2020-12 when no dialect is declared.
func (p *Projection) Standalone() ([]byte, error) {
	return p.StandaloneWithOptions(StandaloneOptions{})
}

// Export a standalone schema using explicit bases and offline resources while preserving declared dialects.
func (p *Projection) StandaloneWithOptions(options StandaloneOptions) ([]byte, error) {
	if p == nil || p.Root == nil {
		return nil, fmt.Errorf("openapi.schema.root: root Schema is required")
	}
	raw, err := json.Marshal(p.Root)
	if err != nil {
		return nil, err
	}
	var root any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err = decoder.Decode(&root); err != nil {
		return nil, err
	}
	obj, ok := root.(map[string]any)
	if !ok {
		obj = map[string]any{"allOf": []any{root}}
	}
	if _, exists := obj["$schema"]; !exists {
		dialect := options.Dialect
		if dialect == "" {
			dialect = "https://json-schema.org/draft/2020-12/schema"
		}
		obj["$schema"] = dialect
	}
	defs, _ := obj["$defs"].(map[string]any)
	if defs == nil {
		defs = map[string]any{}
	}
	names := make([]string, 0, len(p.Components))
	for name := range p.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := p.Components[name]
		if s == nil {
			return nil, fmt.Errorf("openapi.schema.component: component must not be nil: %s", name)
		}
		if _, exists := defs[name]; exists {
			return nil, fmt.Errorf("openapi.schema.defs.conflict: existing definition conflicts with component name: %s", name)
		}
		b, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		var v any
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if err = dec.Decode(&v); err != nil {
			return nil, err
		}
		defs[name] = v
	}
	obj["$defs"] = defs
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	encoded, issues := validate.Standalone(encoded, names, validate.Options{BaseURI: options.BaseURI, Resources: options.Resources, MaxBytes: options.MaxBytes, MaxResources: options.MaxResources, MaxReferences: options.MaxReferences, MaxIndexBytes: options.MaxIndexBytes, MaxNormalizedBytes: options.MaxNormalizedBytes})
	if len(issues) > 0 {
		return nil, fmt.Errorf("openapi.schema.export: %s %s: %s", issues[0].Code, issues[0].Path, issues[0].Message)
	}
	return encoded, nil
}

// Build a union that permits explicit null.
func nullable(s *spec.Schema) *spec.Schema {
	if s.SchemaObject != nil && len(s.Type) > 0 {
		for _, kind := range s.Type {
			if kind == "null" {
				return s
			}
		}
		s.Type = append(s.Type, "null")
		return s
	}
	return &spec.Schema{SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{s, spec.Typed("null")}}}
}

// Project recursively according to type identity and encoding rules.
func (p *projector) projectType(t types.Type) (*spec.Schema, error) {
	p.count++
	if p.count > p.request.MaxTypes {
		return nil, fmt.Errorf("openapi.schema.budget: type graph exceeds the budget")
	}
	for index, mapper := range p.request.Mappers {
		request := p.request
		request.Type = t
		// Reporting must not alter the inputs of user-supplied projection rules.
		request.Explain = false
		if s, ok, err := mapper(request); ok || err != nil {
			p.recordRule(t, fmt.Sprintf("openapi.TypeMapper[%d]", index), "declared")
			if err != nil {
				return nil, err
			}
			if s == nil {
				return nil, fmt.Errorf("openapi.mapper.invalid: type mapper reported the type as handled without a Schema")
			}
			// Keep pointer projection, annotations, and consumers from modifying mapper-owned data.
			return copyWireSchema(s)
		}
	}
	if codec, ok := p.request.Codec.(WireTypeCodec); ok {
		request := p.request
		request.Type = t
		// Reporting must not alter the inputs of user-supplied projection rules.
		request.Explain = false
		schema, handled, err := codec.ProjectType(request, p.projectType)
		if err != nil {
			return nil, err
		}
		if handled {
			p.recordRule(t, p.request.Codec.Name()+".ProjectType", "declared")
			if schema == nil {
				return nil, fmt.Errorf("openapi.codec.invalid: type rule reported the type as handled without a Schema")
			}
			return copyWireSchema(schema)
		}
	}
	p.recordRule(t, "go.types", "derived")
	if alias, ok := t.(*types.Alias); ok && alias.Obj().Pkg() != nil {
		doc, err := p.project.metadata(alias.Obj())
		if err != nil {
			return nil, err
		}
		if closedEnum(doc) {
			return nil, p.project.annotationIssue(fmt.Errorf("openapi.schema.alias-enum: alias %s has no distinct Go constant type; declare an explicit enum array or provide a TypeMapper", alias.Obj().Name()), alias.Obj())
		}
		base, err := p.projectType(alias.Rhs())
		if err != nil {
			return nil, err
		}
		if doc.Summary == "" && len(doc.Directives) == 0 {
			return base, nil
		}
		// Conjoin alias declarations without replacing inherited constraints or modifying shared components.
		s := &spec.Schema{SchemaObject: &spec.SchemaObject{AllOf: []*spec.Schema{base}}}
		if err = p.annotate(s, doc, false, alias.Obj().Name(), alias.Obj()); err != nil {
			return nil, err
		}
		s.Title = types.TypeString(alias, func(*types.Package) string { return "" })
		p.recordOrigin(alias.Obj(), alias, s, "type", "", false)
		return s, nil
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		s, err := p.projectType(ptr.Elem())
		if err != nil {
			return nil, err
		}
		return nullable(s), nil
	}
	if named, ok := t.(*types.Named); ok {
		identity := types.TypeString(named, func(pkg *types.Package) string { return pkg.Path() })
		_, ownsTypes := p.request.Codec.(WireTypeCodec)
		if !ownsTypes {
			switch identity {
			case "time.Time":
				s := spec.Typed("string")
				s.Format = "date-time"
				return s, nil
			case "time.Duration":
				s := spec.Typed("integer")
				s.Format = "int64"
				return s, nil
			case "encoding/json.Number":
				return spec.Typed("number"), nil
			case "encoding/json.RawMessage", "encoding/json/jsontext.Value":
				return spec.Boolean(true), nil
			}
			if method := jsonValueCodecMethod(t, p.request.Direction); method != "" {
				return nil, fmt.Errorf("openapi.codec.custom: %s defines %s; provide a direction-specific TypeMapper", identity, method)
			}
		}
		codec := "std-json"
		if p.request.Codec != nil {
			codec = p.request.Codec.Name()
		}
		key := identity + "|" + string(p.request.Direction) + "|" + p.request.MediaType + "|" + codec
		if name, ok := p.seen[key]; ok {
			return &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/" + name}}, nil
		}
		sum := sha256.Sum256([]byte(key))
		name := named.Obj().Name() + "_" + hex.EncodeToString(sum[:6])
		p.seen[key] = name
		s, err := p.projectType(named.Underlying())
		if err != nil {
			return nil, err
		}
		doc, err := p.project.metadata(named.Obj())
		if err != nil {
			return nil, err
		}
		if err = p.annotate(s, doc, false, named.Obj().Name(), named.Obj()); err != nil {
			return nil, err
		}
		if closedEnum(doc) {
			if err := p.annotateEnum(s, t); err != nil {
				return nil, err
			}
		}
		// Use title for display while component keys distinguish types and projections.
		if s.SchemaObject != nil && s.Title == "" {
			s.Title = types.TypeString(named, func(*types.Package) string { return "" })
		}
		p.components[name] = s
		p.recordOrigin(named.Obj(), named, s, "type", "", false)
		return &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/" + name}}, nil
	}
	switch x := t.(type) {
	case *types.Basic:
		switch {
		case x.Info()&types.IsBoolean != 0:
			return spec.Typed("boolean"), nil
		case x.Info()&types.IsString != 0:
			return spec.Typed("string"), nil
		case x.Info()&types.IsInteger != 0:
			s := spec.Typed("integer")
			if x.Kind() == types.Int64 {
				s.Format = "int64"
			} else if x.Kind() == types.Uint64 {
				s.Format = "uint64"
			}
			if x.Info()&types.IsUnsigned != 0 {
				s.Minimum = spec.Set(json.Number("0"))
			}
			return s, nil
		case x.Info()&types.IsFloat != 0:
			return spec.Typed("number"), nil
		case x.Kind() == types.UntypedNil:
			return spec.Typed("null"), nil
		}
	case *types.Slice:
		if basic, ok := types.Unalias(x.Elem()).Underlying().(*types.Basic); ok && basic.Kind() == types.Byte &&
			(p.request.Direction == Input || jsonValueCodecMethod(x.Elem(), Output) == "") {
			if err := p.checkOpaqueByteElement(x.Elem()); err != nil {
				return nil, err
			}
			s := spec.Typed("string")
			s.ContentEncoding = "base64"
			return nullable(s), nil
		}
		item, err := p.projectType(x.Elem())
		if err != nil {
			return nil, err
		}
		s := spec.Typed("array")
		s.Items = item
		return nullable(s), nil
	case *types.Array:
		item, err := p.projectType(x.Elem())
		if err != nil {
			return nil, err
		}
		s := spec.Typed("array")
		s.Items = item
		s.MinItems = spec.Set(uint64(x.Len()))
		s.MaxItems = spec.Set(uint64(x.Len()))
		return s, nil
	case *types.Map:
		key := types.Unalias(x.Key())
		basic, basicKey := key.Underlying().(*types.Basic)
		methods, receiver := []string{"AppendText", "MarshalText"}, key
		if p.request.Direction == Input {
			methods, receiver = []string{"UnmarshalText"}, types.NewPointer(key)
		}
		// Match key-specific text receivers; JSON value methods do not encode object property names.
		for _, method := range methods {
			if jsonCodecMethod(receiver, method) {
				return nil, fmt.Errorf("openapi.codec.mapkey: %s uses %s; provide a TypeMapper for the containing map or an explicit WireTypeCodec", x.Key(), method)
			}
		}
		if !basicKey || basic.Info()&(types.IsString|types.IsInteger) == 0 {
			return nil, fmt.Errorf("openapi.codec.mapkey: unknown JSON object key encoding %s", x.Key())
		}
		value, err := p.projectType(x.Elem())
		if err != nil {
			return nil, err
		}
		s := spec.Typed("object")
		s.AdditionalProperties = value
		if basic.Info()&types.IsInteger != 0 {
			s.PropertyNames = spec.Typed("string")
			s.PropertyNames.Pattern = "^-?[0-9]+$"
		}
		return nullable(s), nil
	case *types.Interface:
		if x.NumMethods() == 0 {
			return spec.Boolean(true), nil
		}
		return nil, fmt.Errorf("openapi.schema.interface: nonempty interface requires a concrete implementation or centralized mapping")
	case *types.Struct:
		var fields []WireField
		var err error
		if p.request.Codec != nil {
			fields, err = p.request.Codec.Fields(x)
		} else {
			fields, err = jsonFields(x)
		}
		if err != nil {
			return nil, err
		}
		s := spec.Typed("object")
		s.Properties = map[string]*spec.Schema{}
		required := []string{}
		for _, f := range fields {
			field, err := p.projectType(f.Field.Type())
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			stringEncoded := f.StringEncoded
			if stringEncoded && p.request.Codec == nil {
				if method := jsonValueCodecMethod(f.Field.Type(), p.request.Direction); method != "" {
					if p.request.Direction == Output && !jsonCodecMethod(f.Field.Type(), method) {
						return nil, fmt.Errorf("openapi.codec.addressability: field %s has a pointer-only %s with ,string; map its containing type or provide an explicit WireTypeCodec", f.Name, method)
					}
					// A custom method owns its declared shape and constraints, including quoted-field use sites.
					stringEncoded = false
				}
			}
			if stringEncoded {
				base := f.Field.Type().Underlying()
				if ptr, ok := base.(*types.Pointer); ok {
					base = ptr.Elem().Underlying()
				}
				basic, ok := base.(*types.Basic)
				if !ok || basic.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) == 0 {
					return nil, fmt.Errorf("openapi.codec.string: unsupported type for ,string")
				}
				field = spec.Typed("string")
				if _, ok := types.Unalias(f.Field.Type()).Underlying().(*types.Pointer); ok {
					field = nullable(field)
				}
			}
			doc, err := p.project.metadata(f.Field)
			if err != nil {
				return nil, err
			}
			if err = p.annotate(field, doc, true, f.Name, f.Field); err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			s.Properties[f.Name] = field
			presence := (p.request.Direction == Output && !f.OmitEmpty && !f.Optional) || flag(doc, "required")
			p.recordOrigin(f.Field, f.Field.Type(), field, "field", f.Name, presence)
			if presence {
				required = append(required, f.Name)
			}
		}
		sort.Strings(required)
		if len(required) > 0 {
			s.Required = spec.Set(required)
		}
		return s, nil
	}
	return nil, fmt.Errorf("openapi.schema.unsupported: cannot encode type %s", t)
}

// Match byte-oriented JSON and text interface signatures without executing their methods.
func jsonCodecMethod(receiver types.Type, name string) bool {
	method := types.NewMethodSet(receiver).Lookup(nil, name)
	if method == nil {
		return false
	}
	signature, ok := method.Obj().Type().(*types.Signature)
	if !ok || signature.Variadic() {
		return false
	}
	bytes := types.NewSlice(types.Typ[types.Byte])
	errorType := types.Universe.Lookup("error").Type()
	switch name {
	case "MarshalJSON", "MarshalText":
		return signature.Params().Len() == 0 && signature.Results().Len() == 2 &&
			types.Identical(signature.Results().At(0).Type(), bytes) && types.Identical(signature.Results().At(1).Type(), errorType)
	case "UnmarshalJSON", "UnmarshalText":
		return signature.Params().Len() == 1 && signature.Results().Len() == 1 &&
			types.Identical(signature.Params().At(0).Type(), bytes) && types.Identical(signature.Results().At(0).Type(), errorType)
	case "AppendText":
		return signature.Params().Len() == 1 && signature.Results().Len() == 2 &&
			types.Identical(signature.Params().At(0).Type(), bytes) && types.Identical(signature.Results().At(0).Type(), bytes) && types.Identical(signature.Results().At(1).Type(), errorType)
	case "MarshalJSONTo", "UnmarshalJSONFrom":
		if signature.Params().Len() != 1 || signature.Results().Len() != 1 || !types.Identical(signature.Results().At(0).Type(), errorType) {
			return false
		}
		pointer, ok := types.Unalias(signature.Params().At(0).Type()).(*types.Pointer)
		if !ok {
			return false
		}
		target, ok := types.Unalias(pointer.Elem()).(*types.Named)
		if !ok || target.Obj().Pkg() == nil || target.Obj().Pkg().Path() != "encoding/json/jsontext" {
			return false
		}
		if name == "MarshalJSONTo" {
			return target.Obj().Name() == "Encoder"
		}
		return target.Obj().Name() == "Decoder"
	}
	return false
}

// Recognize direction-specific custom value codecs, preferring JSON over Text across effective receivers.
func jsonValueCodecMethod(t types.Type, direction Direction) string {
	methods := []string{"MarshalJSONTo", "MarshalJSON", "AppendText", "MarshalText"}
	if direction == Input {
		methods = []string{"UnmarshalJSONFrom", "UnmarshalJSON", "UnmarshalText"}
	}
	for _, method := range methods {
		for _, receiver := range []types.Type{t, types.NewPointer(t)} {
			if jsonCodecMethod(receiver, method) {
				return method
			}
		}
	}
	return ""
}

// Recognize bare flags and explicitly true directive values.
func flag(doc comment.Document, key string) bool {
	for _, d := range doc.Directives {
		if string(d.Values[key]) == "true" {
			return true
		}
	}
	return false
}

// Only a type-level enum flag closes the set of declared constants.
func closedEnum(doc comment.Document) bool { return flag(doc, "enum") }

// Apply contract annotations without overriding structural facts.
func (p *projector) annotate(s *spec.Schema, doc comment.Document, field bool, site string, object types.Object) (failure error) {
	defer func() {
		if failure != nil {
			failure = p.project.annotationIssue(failure, object)
		}
	}()
	if doc.Summary == "" && len(doc.Directives) == 0 {
		return nil
	}
	if s.Bool != nil {
		if !*s.Bool {
			return fmt.Errorf("openapi.schema.annotation: structural annotations cannot be added to an unsatisfiable Schema")
		}
		s.Bool = nil
		s.SchemaObject = &spec.SchemaObject{}
	}
	if s.SchemaObject == nil {
		s.SchemaObject = &spec.SchemaObject{}
	}
	s.Description = strings.TrimSpace(doc.Summary + "\n\n" + doc.Description)
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var before spec.Schema
	if err = json.Unmarshal(raw, &before); err != nil {
		return err
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	for _, d := range doc.Directives {
		if d.Kind != "" {
			return fmt.Errorf("openapi.comment.context: request and response declarations cannot apply to fields or types")
		}
		for _, k := range directiveKeys(d.Values) {
			v := d.Values[k]
			if k == "required" || k == "nonnull" || k == "nullable" || k == "ignore" {
				if string(v) != "true" && string(v) != "false" {
					return fmt.Errorf("openapi.comment.value: %s must be a boolean", k)
				}
			}
			switch k {
			case "required":
				if !field {
					return fmt.Errorf("openapi.comment.context: required only applies to fields")
				}
				continue
			case "nonnull":
				// Apply null exclusion after all directives so later constraints cannot overwrite it.
				continue
			case "nullable":
				if string(v) == "true" {
					if flag(doc, "nonnull") {
						return fmt.Errorf("openapi.comment.conflict: nullable conflicts with nonnull")
					}
					if len(s.Type) > 0 {
						nullable(s)
						obj["type"], _ = json.Marshal(s.Type)
					} else {
						return fmt.Errorf("openapi.schema.nullable: this reference union requires a centralized type extension")
					}
				}
				continue
			case "enum":
				if string(v) == "true" {
					if field {
						return fmt.Errorf("openapi.comment.enum: field enum must be a JSON array")
					}
					continue
				}
			case "ignore":
				if string(v) == "true" {
					return fmt.Errorf("openapi.schema.ignore: field is still transmitted; actual fields cannot be hidden")
				}
				continue
			case "operationId", "tags", "status", "mediaType", "type":
				return fmt.Errorf("openapi.comment.context: %s is not applicable here", k)
			case "writeOnly":
				if p.request.Direction == Output && string(v) == "true" {
					return fmt.Errorf("openapi.schema.writeOnly: field is still emitted by the actual encoder")
				}
			}
			obj[k] = v
		}
	}
	if flag(doc, "nonnull") {
		if encodedType, exists := obj["type"]; exists {
			var kinds spec.Types
			if err = json.Unmarshal(encodedType, &kinds); err != nil {
				return err
			}
			nonnull := spec.Types{}
			for _, kind := range kinds {
				if kind != "null" {
					nonnull = append(nonnull, kind)
				}
			}
			if len(nonnull) == 0 {
				return fmt.Errorf("openapi.comment.conflict: nonnull contradicts a null-only wire type")
			}
			obj["type"], err = json.Marshal(nonnull)
			if err != nil {
				return err
			}
		} else if _, exists := obj["not"]; !exists {
			obj["not"] = json.RawMessage(`{"type":"null"}`)
		} else {
			// Keep an existing negation and conjoin null exclusion instead of replacing it.
			var constraints []json.RawMessage
			if previous, exists := obj["allOf"]; exists {
				if err = json.Unmarshal(previous, &constraints); err != nil {
					return err
				}
			}
			constraints = append(constraints, json.RawMessage(`{"not":{"type":"null"}}`))
			obj["allOf"], err = json.Marshal(constraints)
			if err != nil {
				return err
			}
		}
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	var values map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if err = decoder.Decode(&values); err != nil {
		return err
	}
	if issues := validate.SchemaKeywordValues(values); len(issues) > 0 {
		return fmt.Errorf("openapi.comment.value: %s %s", issues[0].Path, issues[0].Message)
	}
	if err = json.Unmarshal(encoded, s); err != nil {
		return fmt.Errorf("openapi.comment.value: %w", err)
	}
	if len(doc.Directives) > 0 {
		p.annotations = append(p.annotations, annotationCheck{schema: s, before: &before, doc: doc, site: site, object: object})
	}
	return nil
}

// Reject scalar declarations that cannot be preserved inside an opaque Base64 representation.
func (p *projector) checkOpaqueByteElement(t types.Type) error {
	for {
		var object *types.TypeName
		var next types.Type
		switch value := t.(type) {
		case *types.Alias:
			object, next = value.Obj(), value.Rhs()
		case *types.Named:
			object = value.Obj()
		default:
			return nil
		}
		p.count++
		if p.count > p.request.MaxTypes {
			return fmt.Errorf("openapi.schema.budget: byte element aliases exceed the type budget")
		}
		doc, err := p.project.metadata(object)
		if err != nil {
			return err
		}
		if len(doc.Directives) > 0 {
			return p.project.annotationIssue(fmt.Errorf("openapi.codec.byte-element: declarations on %s cannot be preserved in opaque Base64; map the containing byte slice or provide an explicit WireTypeCodec", object.Name()), object)
		}
		if next == nil {
			return nil
		}
		t = next
	}
}

// Match standard JSON empty-value omission without confusing it with zero-value omission.
func jsonEmptyValuePossible(t types.Type) bool {
	switch x := types.Unalias(t).Underlying().(type) {
	case *types.Array:
		return x.Len() == 0
	case *types.Slice, *types.Map, *types.Pointer, *types.Interface:
		return true
	case *types.Basic:
		return x.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) != 0
	}
	return false
}

// Standard JSON ignores the string option on composite kinds instead of converting their wire representation.
func jsonStringOptionApplies(t types.Type) bool {
	t = types.Unalias(t).Underlying()
	if pointer, ok := t.(*types.Pointer); ok {
		t = types.Unalias(pointer.Elem()).Underlying()
	}
	basic, ok := t.(*types.Basic)
	return ok && basic.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) != 0
}

// Select JSON fields by embedding depth, tag priority, and conflict rules.
func jsonFields(root *types.Struct) ([]WireField, error) {
	type candidate struct {
		wire   WireField
		depth  int
		tagged bool
	}
	type level struct {
		typ       *types.Struct
		depth     int
		optional  bool
		ancestors map[*types.Struct]bool
	}
	queue := []level{{typ: root, ancestors: map[*types.Struct]bool{root: true}}}
	all := map[string][]candidate{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for i := 0; i < current.typ.NumFields(); i++ {
			f := current.typ.Field(i)
			ft := types.Unalias(f.Type())
			optional := current.optional
			if ptr, ok := ft.(*types.Pointer); ok {
				optional = true
				ft = types.Unalias(ptr.Elem())
			}
			under := ft.Underlying()
			embeddedStruct, isStruct := under.(*types.Struct)
			if !f.Exported() && (!f.Embedded() || !isStruct) {
				continue
			}
			tag := reflect.StructTag(current.typ.Tag(i)).Get("json")
			if tag == "-" {
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			if strings.ContainsAny(name, "\\'\"`") {
				return nil, fmt.Errorf("openapi.codec.fieldname: field %s has a reserved-character JSON tag name; provide an explicit WireCodec for the actual encoder's name selection", f.Name())
			}
			tagged := name != ""
			omit, stringEncoded := false, false
			for _, option := range parts[1:] {
				if option == "format" || strings.HasPrefix(option, "format:") {
					return nil, fmt.Errorf("openapi.codec.format: field %s uses a format tag unsupported by the standard JSON compatibility profile; provide an explicit WireCodec for a different encoder", f.Name())
				}
				omit = omit || option == "omitzero" || option == "omitempty" && jsonEmptyValuePossible(f.Type())
				stringEncoded = stringEncoded || option == "string" && jsonStringOptionApplies(f.Type())
			}
			if f.Embedded() && isStruct && !tagged {
				if current.ancestors[embeddedStruct] {
					continue
				}
				ancestors := map[*types.Struct]bool{}
				for k, v := range current.ancestors {
					ancestors[k] = v
				}
				ancestors[embeddedStruct] = true
				queue = append(queue, level{typ: embeddedStruct, depth: current.depth + 1, optional: optional, ancestors: ancestors})
				continue
			}
			if name == "" {
				name = f.Name()
			}
			all[name] = append(all[name], candidate{wire: WireField{Name: name, Field: f, OmitEmpty: omit, StringEncoded: stringEncoded, Optional: current.optional}, depth: current.depth, tagged: tagged})
		}
	}
	out := []WireField{}
	for _, group := range all {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].depth != group[j].depth {
				return group[i].depth < group[j].depth
			}
			return group[i].tagged && !group[j].tagged
		})
		if len(group) > 1 && group[0].depth == group[1].depth && group[0].tagged == group[1].tagged {
			continue
		}
		out = append(out, group[0].wire)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
