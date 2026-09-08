package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"sort"
	"strings"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/spec"
)

// Represent a propagated static value without inventing unknown payloads.
type Value struct {
	// Track a local address during analysis for alias writes and external-call invalidation.
	address uint64
	// Function implementations and captured identities exist only during compilation, never in Bundle.
	callable *functionValue
	// Preserve resolved standard-library symbol identity for explicit binders and aliases.
	Object   types.Object
	Type     types.Type
	Constant constant.Value
	Fields   map[string]Value
	Nil      bool
	// A non-nil interface contains a known nil pointer or collection without changing interface comparisons.
	DynamicNil bool
	// Mark a boxed concrete value and separately preserve its definite non-nil identity.
	Boxed         bool
	DynamicNonNil bool
	// Mark a definitely non-nil Go result for conditional propagation.
	NonNil  bool
	Unknown bool
}

// Represent a framework-neutral frontend effect.
type EffectKind string

// Describe wire facts without framework method names.
const (
	RequestBody     EffectKind = "requestBody"
	RequestField    EffectKind = "requestField"
	ParameterObject EffectKind = "parameterObject"
	ParameterRead   EffectKind = "parameter"
	ResponseBody    EffectKind = "responseBody"
	// Consecutive items of one media type constrain itemSchema without claiming stream length or item order.
	ResponseItem   EffectKind = "responseItem"
	ResponseStatus EffectKind = "status"
	ResponseCommit EffectKind = "commit"
	Handled        EffectKind = "handled"
	ResponseHeader EffectKind = "header"
	Abort          EffectKind = "abort"
	Unresolved     EffectKind = "unresolved"
)

// Store a response header value and provenance for commit snapshots and reports.
type HeaderValue struct {
	Value  Value
	Source openapi.Source
}

// Record request, response, status, and control effects with their sources.
type Effect struct {
	// This outcome cannot occur with an empty request body; only RequestBody and RequestField may carry this proof.
	NonEmptyBody bool
	// Select the response payload codec media type independently of its outer protocol representation.
	PayloadMediaType string
	// Wrap a projected response schema at compile time; both sides are detached and nil/errors prevent trusted publication.
	TransformSchema func(*spec.Schema) (*spec.Schema, error)

	// Describe one body field's wire encoding; Required constrains field presence only.
	Encoding *spec.Encoding

	// One logical input may come from multiple locations, so cross-location presence requirements cannot be dropped.
	AlternativeLocations bool
	// Frontends explicitly provide parameter serialization; the core does not infer framework rules.
	Style   string
	Explode spec.Optional[bool]
	// Supply an explicit request or response wire representation; omission uses actual type projection.
	WireSchema *spec.Schema
	// Describe header replacement, removal, and insertion only when the current value is empty.
	DeleteHeader  bool
	HeaderIfEmpty bool
	// Record response headers and provenance at commit time.
	Headers   map[string]HeaderValue
	Kind      EffectKind
	Name      string
	In        string
	Status    string
	MediaType string
	Payload   Value
	Required  bool
	Codec     WireCodec
	Source    openapi.Source
	Message   string
	Fix       string
}

// Expose standard-library call views and propagated arguments without third-party SSA.
type CallContext struct {
	// Retain the resolved function value without exposing internal capture cells to frontends.
	callee *functionValue
	// A path-local response snapshot lets frontends select actual rendering behavior.
	Response  ResponseState
	Function  Function
	Call      *ast.CallExpr
	Object    *types.Func
	Arguments []Value
	Receiver  Value
	Source    openapi.Source
}

// Associate one call result tuple with its co-occurring effects as a finite alternative.
type CallOutcome struct {
	// Apply this result alternative only under the finite request condition.
	When    openapi.RequestCondition
	Results []Value
	Effects []Effect
}

// Support frontends whose handlers return responses or errors.
type ReturnContext struct {
	Function Function
	Values   []Value
	Source   openapi.Source
}

// Register deterministic frontend rules; callbacks may be reused through parameterized helper summaries.
type Frontend struct {
	// Declare synchronous callback invocation and repetition while the core executes neutral control flow.
	Callback func(CallContext) (*CallbackPlan, error)
	// Try finite call alternatives first; an empty set falls back to Call and errors prevent trusted publication.
	CallOutcomes   func(CallContext) ([]CallOutcome, error)
	Name           string
	Match          func(Function) bool
	Entry          func(Function) []Effect
	Call           func(CallContext) ([]Effect, error)
	Return         func(ReturnContext) ([]Effect, error)
	CarriesEffects func(types.Type) bool
}

// Configure analysis budgets, frontend dispatch, and centralized type mappings.
type Options struct {
	// Bound parameterized helper contexts and normalized retained summary bytes per candidate.
	MaxSummaries, MaxSummaryBytes int
	// Reanalyze every helper invocation for differential verification or troubleshooting.
	DisableHelperSummaries bool
	// Capture optional explanations; zero uses a sixteen-MiB evidence budget.
	Explain         bool
	MaxExplainBytes int
	// Declare stable JSON inputs for custom mapping or captured callback configuration without serializing function addresses.
	Configuration map[string]json.RawMessage
	Load          LoadOptions
	Frontends     []Frontend
	MaxDepth      int
	MaxPaths      int
	MaxCalls      int
	// Maximum analyzed iterations for each synchronous repeated invocation.
	MaxIterations int
	Mappers       []TypeMapper
}

// Store a writable Bundle and its compilation report.
type Result struct {
	explanations *explanationCapture
	Bundle       openapi.Bundle
	Report       openapi.Report
}

// Compile real projects with registered frontends and shared annotations and projections.
func Compile(ctx context.Context, options Options) (*Result, error) {
	if len(options.Frontends) == 0 {
		return nil, fmt.Errorf("openapi.frontend.missing: register at least one frontend explicitly")
	}
	names := map[string]bool{}
	for _, f := range options.Frontends {
		if f.Name == "" || f.Match == nil || names[f.Name] {
			return nil, fmt.Errorf("openapi.frontend.invalid: invalid name or candidate rule")
		}
		names[f.Name] = true
	}
	if len(options.Mappers) > 0 && len(options.Configuration) == 0 {
		return nil, fmt.Errorf("openapi.fingerprint.configuration: custom TypeMapper must declare Configuration inputs")
	}
	configuration, err := configurationDigest(options.Configuration)
	if err != nil {
		return nil, err
	}
	project, err := Load(ctx, options.Load)
	if err != nil {
		return nil, err
	}
	if options.MaxDepth == 0 {
		options.MaxDepth = 12
	}
	if options.MaxPaths == 0 {
		options.MaxPaths = 128
	}
	if options.MaxCalls == 0 {
		options.MaxCalls = 10000
	}
	if options.MaxSummaries == 0 {
		options.MaxSummaries = 512
	}
	if options.MaxSummaryBytes == 0 {
		options.MaxSummaryBytes = 16 << 20
	}
	if options.MaxIterations == 0 {
		options.MaxIterations = 32
	}
	if options.MaxDepth < 1 || options.MaxPaths < 1 || options.MaxCalls < 1 || options.MaxIterations < 1 || options.MaxSummaries < 1 || options.MaxSummaryBytes < 1 {
		return nil, fmt.Errorf("openapi.analysis.budget: all budgets must be positive")
	}
	if options.MaxExplainBytes < 0 {
		return nil, fmt.Errorf("openapi.explain.budget: MaxExplainBytes must not be negative")
	}
	if options.Explain {
		limit := options.MaxExplainBytes
		if limit == 0 {
			limit = 16 << 20
		}
		project.explanations = &explanationCapture{maxBytes: limit, known: map[string]openapi.Source{}, declarations: map[string][]DeclarationEvidence{}}
	}
	data := openapi.BundleData{FormatVersion: openapi.BundleFormatVersion, SpecVersion: "3.2.0", Capabilities: []string{"oas32", "schema2020-12"}, Components: spec.Components{Schemas: map[string]*spec.Schema{}}, Profile: project.inputs.profile}
	codecs := map[string]bool{}
	for _, fn := range project.Functions() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var matched []Frontend
		for _, front := range options.Frontends {
			if front.Match(fn) {
				matched = append(matched, front)
			}
		}
		if len(matched) == 0 {
			continue
		}
		if len(matched) > 1 {
			return nil, fmt.Errorf("openapi.frontend.ambiguous: %s matches multiple frontends", fn.Symbol)
		}
		front := matched[0]
		a := analyzer{ctx: ctx, project: project, options: options, frontend: front}
		initial := flow{values: map[uint64]Value{}, bindings: map[types.Object]uint64{}}
		for i := 0; i < fn.Signature.Params().Len(); i++ {
			param := fn.Signature.Params().At(i)
			a.bind(&initial, param, Value{Type: param.Type()})
		}
		if front.Entry != nil {
			a.effects(&initial, front.Entry(fn))
		}
		var paths []flow
		if fn.Declaration.Body == nil {
			a.unknown(&initial, fn.Source, "candidate has no analyzable function body")
			paths = []flow{initial}
		} else {
			paths = a.statements(fn, fn.Declaration.Body.List, []flow{initial}, 0, true)
		}
		// Finalize a pending bodyless status only after all handler statements finish.
		for i := range paths {
			a.finishResponse(&paths[i])
		}
		template := openapi.Template{Key: openapi.OperationKey(fn.Symbol), Symbol: fn.Symbol, Source: fn.Source, Operation: spec.Operation{Responses: map[string]spec.RefOr[spec.Response]{}}}
		if fn.Signature.Recv() == nil {
			symbol := fn.Symbol
			template.RuntimeSymbols = []string{symbol}
			if fn.Package.Name == "main" {
				// Preserve the complete symbols observed in main binaries and go test.
				template.RuntimeSymbols = append(template.RuntimeSymbols, "main."+fn.Object.Name())
			}
		}
		doc := project.comments[fn.Object]
		template.Operation.Summary = doc.Summary
		template.Operation.Description = doc.Description
		for _, d := range doc.Directives {
			if d.Kind != "" {
				continue
			}
			for k, v := range d.Values {
				switch k {
				case "operationId":
					err = json.Unmarshal(v, &template.Operation.OperationID)
				case "tags":
					err = json.Unmarshal(v, &template.Operation.Tags)
				case "deprecated":
					err = json.Unmarshal(v, &template.Operation.Deprecated)
				default:
					err = fmt.Errorf("function comments do not accept %s", k)
				}
				if err != nil {
					return nil, fmt.Errorf("openapi.comment.context: %w", err)
				}
			}
		}
		template.Diagnostics = append(template.Diagnostics, a.diagnostics...)
		for _, path := range paths {
			for _, effect := range path.effects {
				if effect.Codec != nil {
					codecs[effect.Codec.Name()] = true
				}
			}
		}
		if project.explanations != nil {
			project.explanations.handler = fn.Symbol
			project.captureDeclarations(fn)
		}
		declarations := project.declarations(fn, &template.Diagnostics)
		project.mergeConditionalPaths(&template, paths, data.Components.Schemas, options.Mappers)
		project.mergeDeclarations(&template, declarations, paths, data.Components.Schemas, options.Mappers)
		data.Templates = append(data.Templates, template)
	}
	project.captureKnownFields()
	if project.explanations != nil && project.explanations.err != nil {
		return nil, project.explanations.err
	}
	data.Profile.Frontend = strings.Join(sortedKeys(names), ",")
	for _, template := range data.Templates {
		if len(template.Variants) > 0 {
			data.Capabilities = append(data.Capabilities, openapi.RequestConditionsCapability)
			break
		}
	}
	data.Profile.Codecs = sortedKeys(codecs)
	data.Profile.ConfigurationDigest = configuration
	data.Fingerprint, err = project.fingerprint(options, data)
	if err != nil {
		return nil, err
	}
	bundle, err := openapi.NewBundle(data)
	if err != nil {
		return nil, err
	}
	if project.explanations != nil {
		project.explanations.bundle = bundle
	}
	return &Result{Bundle: bundle, Report: openapi.Report{Diagnostics: []openapi.Diagnostic{}}, explanations: project.explanations}, nil
}

// Return string keys in stable order.
func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Deduplicate alternatives and use anyOf when their shapes may overlap.
func union(a, b *spec.Schema) *spec.Schema {
	if a == nil {
		return b
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) == string(y) {
		return a
	}
	if a.SchemaObject != nil && len(a.AnyOf) > 0 {
		for _, existing := range a.AnyOf {
			raw, _ := json.Marshal(existing)
			if string(raw) == string(y) {
				return a
			}
		}
		a.AnyOf = append(a.AnyOf, b)
		return a
	}
	return &spec.Schema{SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{a, b}}}
}

// Project propagated literals or real types into shared Schemas.
func (p *Project) valueSchema(v Value, direction Direction, media string, codec WireCodec, mappers []TypeMapper, components map[string]*spec.Schema, site SchemaUse) (*spec.Schema, error) {
	projection, err := p.projectValue(v, direction, media, codec, mappers)
	p.captureProjection(site, projection, err)
	if err != nil {
		return nil, err
	}
	for name, schema := range projection.Components {
		components[name] = schema
	}
	return projection.Root, nil
}

// Project a known value once, retaining literal-map facts and optional nested origins.
func (p *Project) projectValue(v Value, direction Direction, media string, codec WireCodec, mappers []TypeMapper) (*Projection, error) {
	if v.Unknown || v.Type == nil {
		return nil, fmt.Errorf("critical payload type is unresolved")
	}
	if v.Nil || v.DynamicNil {
		return &Projection{Root: spec.Typed("null")}, nil
	}
	if _, isMap := v.Type.Underlying().(*types.Map); v.Fields != nil && isMap {
		projection := &Projection{Root: spec.Typed("object"), Components: map[string]*spec.Schema{}}
		projection.Root.Properties = map[string]*spec.Schema{}
		for _, name := range sortedKeys(v.Fields) {
			child, err := p.projectValue(v.Fields[name], direction, media, codec, mappers)
			if err != nil {
				return nil, err
			}
			projection.Root.Properties[name] = child.Root
			for name, schema := range child.Components {
				projection.Components[name] = schema
			}
			projection.Origins = append(projection.Origins, child.Origins...)
			projection.Rules = append(projection.Rules, child.Rules...)
			projection.Audit = append(projection.Audit, child.Audit...)
		}
		projection.Root.Required = spec.Set(sortedKeys(v.Fields))
		return projection, nil
	}
	return p.Schema(ProjectionRequest{Type: v.Type, Direction: direction, MediaType: media, Codec: codec, Mappers: mappers, Explain: p.explanations != nil})
}

// Link neutral effects without disguising unknown responses as default.
func (p *Project) mergeEffect(op *spec.Operation, e Effect, components map[string]*spec.Schema, mappers []TypeMapper) error {
	switch e.Kind {
	case Unresolved:
		return fmt.Errorf("%s; %s", e.Message, e.Fix)
	case ParameterObject:
		return p.mergeParameterObject(op, e, components, mappers)
	case ParameterRead:
		return p.mergeParameterRead(op, e, components, mappers)
	case ResponseBody, ResponseItem, ResponseStatus:
		if e.Status == "" {
			return fmt.Errorf("response status is not an evaluable constant")
		}
		response := op.Responses[e.Status]
		if response.Value == nil {
			response = spec.Inline(spec.Response{Description: "Response " + e.Status, Content: map[string]spec.RefOr[spec.MediaType]{}})
		}
		if e.Kind == ResponseBody || e.Kind == ResponseItem {
			if e.MediaType == "" {
				return fmt.Errorf("response media type is unresolved")
			}
			schema, err := p.responseSchema(e, components, mappers)
			if err != nil {
				return err
			}
			media := response.Value.Content[e.MediaType]
			if media.Value == nil {
				media = spec.Inline(spec.MediaType{})
			}
			if e.Kind == ResponseItem {
				if media.Value.Schema != nil {
					return fmt.Errorf("whole-body and item-wise responses use the same media type; framing cannot be merged")
				}
				media.Value.ItemSchema = union(media.Value.ItemSchema, schema)
			} else {
				if media.Value.ItemSchema != nil {
					return fmt.Errorf("whole-body and item-wise responses use the same media type; framing cannot be merged")
				}
				media.Value.Schema = union(media.Value.Schema, schema)
			}
			response.Value.Content[e.MediaType] = media
		}
		if err := mergeResponseHeaders(response.Value, e.Headers); err != nil {
			return err
		}
		op.Responses[e.Status] = response
	case ResponseHeader, Abort:
		// The analyzer handles commits and termination without inventing responses.
	default:
		return fmt.Errorf("unknown frontend effect %s", e.Kind)
	}
	return nil
}
