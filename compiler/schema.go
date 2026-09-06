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

	"github.com/openapi-golang/openapi/internal/comment"
	"github.com/openapi-golang/openapi/internal/validate"
	"github.com/openapi-golang/openapi/spec"
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
	Type      types.Type
	Direction Direction
	MediaType string
	Codec     WireCodec
	Mappers   []TypeMapper
	MaxTypes  int
}

// Return a root Schema and its complete component closure.
type Projection struct {
	Root       *spec.Schema
	Components map[string]*spec.Schema
	Audit      []string
}

// Keep a private recursion cache for one projection.
type projector struct {
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
	return &Projection{Root: root, Components: pr.components, Audit: audit}, nil
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
	for _, mapper := range p.request.Mappers {
		request := p.request
		request.Type = t
		if s, ok, err := mapper(request); ok || err != nil {
			return s, err
		}
	}
	if codec, ok := p.request.Codec.(WireTypeCodec); ok {
		request := p.request
		request.Type = t
		schema, handled, err := codec.ProjectType(request, p.projectType)
		if err != nil {
			return nil, err
		}
		if handled {
			if schema == nil {
				return nil, fmt.Errorf("openapi.codec.invalid: type rule reported the type as handled without a Schema")
			}
			return copyWireSchema(schema)
		}
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
			method := "MarshalJSON"
			textMethod := "MarshalText"
			if p.request.Direction == Input {
				method = "UnmarshalJSON"
				textMethod = "UnmarshalText"
			}
			for _, mt := range []types.Type{t, types.NewPointer(t)} {
				set := types.NewMethodSet(mt)
				for i := 0; i < set.Len(); i++ {
					name := set.At(i).Obj().Name()
					if name == method || name == textMethod {
						return nil, fmt.Errorf("openapi.codec.custom: %s defines %s; provide a direction-specific TypeMapper", identity, name)
					}
				}
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
		if err = p.annotate(s, p.project.comments[named.Obj()], false, named.Obj().Name()); err != nil {
			return nil, err
		}
		if doc := p.project.comments[named.Obj()]; closedEnum(doc) {
			if err := p.annotateEnum(s, t); err != nil {
				return nil, err
			}
		}
		// Use title for display while component keys distinguish types and projections.
		if s.SchemaObject != nil && s.Title == "" {
			s.Title = types.TypeString(named, func(*types.Package) string { return "" })
		}
		p.components[name] = s
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
			if x.Kind() == types.Int64 || x.Kind() == types.Uint64 {
				s.Format = "int64"
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
		if basic, ok := types.Unalias(x.Elem()).(*types.Basic); ok && basic.Kind() == types.Byte {
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
		if named, ok := key.(*types.Named); ok {
			key = named.Underlying()
		}
		basic, ok := key.(*types.Basic)
		if !ok || basic.Info()&(types.IsString|types.IsInteger) == 0 {
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
			if f.StringEncoded {
				base := f.Field.Type().Underlying()
				if ptr, ok := base.(*types.Pointer); ok {
					base = ptr.Elem().Underlying()
				}
				basic, ok := base.(*types.Basic)
				if !ok || basic.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) == 0 {
					return nil, fmt.Errorf("openapi.codec.string: unsupported type for ,string")
				}
				field = spec.Typed("string")
				if _, ok := f.Field.Type().(*types.Pointer); ok {
					field = nullable(field)
				}
			}
			doc := p.project.comments[f.Field]
			if err = p.annotate(field, doc, true, f.Name); err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			s.Properties[f.Name] = field
			if (p.request.Direction == Output && !f.OmitEmpty && !f.Optional) || flag(doc, "required") {
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
func (p *projector) annotate(s *spec.Schema, doc comment.Document, field bool, site string) error {
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
				if string(v) == "true" {
					if len(s.Type) > 0 {
						types := spec.Types{}
						for _, t := range s.Type {
							if t != "null" {
								types = append(types, t)
							}
						}
						s.Type = types
						encoded, _ := json.Marshal(types)
						obj["type"] = encoded
					}
				}
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
		p.annotations = append(p.annotations, annotationCheck{schema: s, before: &before, doc: doc, site: site})
	}
	return nil
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
			tagged := name != ""
			omit, stringEncoded := false, false
			for _, option := range parts[1:] {
				omit = omit || option == "omitempty" || option == "omitzero"
				stringEncoded = stringEncoded || option == "string"
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
