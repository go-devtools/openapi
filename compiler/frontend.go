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

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Represent a propagated static value without inventing unknown payloads.
// 表示有限传播的静态值；未知值从不伪装成具体 payload。
type Value struct {
	// Track a local address during analysis for alias writes and external-call invalidation.
	// 保存局部变量地址的分析期身份，用于别名写入和外部调用失效处理。
	address uint64
	// Function implementations and captured identities exist only during compilation, never in Bundle.
	// 函数实现与捕获身份仅在编译期传播，不写入 Bundle。
	callable *functionValue
	// Preserve resolved standard-library symbol identity for explicit binders and aliases.
	// 保留已解析的标准库符号身份，供前端识别显式绑定器和别名。
	Object   types.Object
	Type     types.Type
	Constant constant.Value
	Fields   map[string]Value
	Nil      bool
	// A non-nil interface contains a known nil pointer or collection without changing interface comparisons.
	// 非 nil 接口内部持有已知 nil 指针或集合；不改变接口自身的比较结果。
	DynamicNil bool
	// Mark a boxed concrete value and separately preserve its definite non-nil identity.
	// 标记已装箱的具体值，并独立保留其确定非 nil 的事实。
	Boxed         bool
	DynamicNonNil bool
	// Mark a definitely non-nil Go result for conditional propagation.
	// 明确表示 Go 层面的非 nil 返回值，用于条件传播。
	NonNil  bool
	Unknown bool
}

// Represent a framework-neutral frontend effect.
// 表示框架前端输出的中立效果。
type EffectKind string

// Describe wire facts without framework method names.
// 共同效果描述网络事实，不包含任何框架方法名。
const (
	RequestBody     EffectKind = "requestBody"
	RequestField    EffectKind = "requestField"
	ParameterObject EffectKind = "parameterObject"
	ParameterRead   EffectKind = "parameter"
	ResponseBody    EffectKind = "responseBody"
	// Consecutive items of one media type constrain itemSchema without claiming stream length or item order.
	// 同媒体类型的连续条目共同约束 itemSchema，不限制流长度或条目顺序。
	ResponseItem   EffectKind = "responseItem"
	ResponseStatus EffectKind = "status"
	ResponseCommit EffectKind = "commit"
	Handled        EffectKind = "handled"
	ResponseHeader EffectKind = "header"
	Abort          EffectKind = "abort"
	Unresolved     EffectKind = "unresolved"
)

// Store a response header value and provenance for commit snapshots and reports.
// 保存响应头值和推导来源，供提交快照与报告共同使用。
type HeaderValue struct {
	Value  Value
	Source openapi.Source
}

// Record request, response, status, and control effects with their sources.
// 记录请求、响应、状态和控制效果及其来源。
type Effect struct {
	// Select the response payload codec media type independently of its outer protocol representation.
	// 指定响应载荷自身的编解码媒体类型，允许外层协议使用不同表示。
	PayloadMediaType string
	// Wrap a projected response schema at compile time; both sides are detached and nil/errors prevent trusted publication.
	// 编译期包装已投影的响应 Schema；输入输出均隔离，nil 或错误阻止可信发布。
	TransformSchema func(*spec.Schema) (*spec.Schema, error)

	// Describe one body field's wire encoding; Required constrains field presence only.
	// 描述单个请求体字段的网络编码；此效果的 Required 只约束字段存在。
	Encoding *spec.Encoding

	// One logical input may come from multiple locations, so cross-location presence requirements cannot be dropped.
	// 同一逻辑输入可能来自多个位置，不能丢弃跨位置必填关系。
	AlternativeLocations bool
	// Frontends explicitly provide parameter serialization; the core does not infer framework rules.
	// 参数序列化由前端明确提供，核心不猜测框架规则。
	Style   string
	Explode spec.Optional[bool]
	// Supply an explicit request or response wire representation; omission uses actual type projection.
	// 明确的请求或响应网络表示由前端提供；省略时复用真实类型投影。
	WireSchema *spec.Schema
	// Describe header replacement, removal, and insertion only when the current value is empty.
	// 响应头的替换、删除及仅在当前值为空时设置语义。
	DeleteHeader  bool
	HeaderIfEmpty bool
	// Record response headers and provenance at commit time.
	// 分析器记录提交时的响应头快照及其来源。
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
// 提供标准库调用视图和核心已传播的实参，不暴露第三方 SSA。
type CallContext struct {
	// Retain the resolved function value without exposing internal capture cells to frontends.
	// 保存本次已解析的函数值，不向前端泄露内部捕获单元。
	callee *functionValue
	// A path-local response snapshot lets frontends select actual rendering behavior.
	// 当前执行路径的响应状态副本，供前端选择实际渲染行为。
	Response  ResponseState
	Function  Function
	Call      *ast.CallExpr
	Object    *types.Func
	Arguments []Value
	Receiver  Value
	Source    openapi.Source
}

// Associate one call result tuple with its co-occurring effects as a finite alternative.
// 将一次调用的返回值与其共同发生的效果绑定为有限备选。
type CallOutcome struct {
	// Apply this result alternative only under the finite request condition.
	// 仅在该有限请求条件下发生此返回备选。
	When    openapi.RequestCondition
	Results []Value
	Effects []Effect
}

// Support frontends whose handlers return responses or errors.
// 支持返回响应值或 error 的前端形态。
type ReturnContext struct {
	Function Function
	Values   []Value
	Source   openapi.Source
}

// Register frontend rules explicitly; absent callbacks define no rules.
// 显式注册前端规则；未提供的回调表示此类入口没有框架规则。
type Frontend struct {
	// Declare synchronous callback invocation and repetition while the core executes neutral control flow.
	// 显式声明同步回调的调用与重复规则，通用控制流仍由核心执行。
	Callback func(CallContext) (*CallbackPlan, error)
	// Try finite call alternatives first; an empty set falls back to Call and errors prevent trusted publication.
	// 优先尝试有限调用备选；空集合回退到 Call，错误阻止可信发布。
	CallOutcomes   func(CallContext) ([]CallOutcome, error)
	Name           string
	Match          func(Function) bool
	Entry          func(Function) []Effect
	Call           func(CallContext) ([]Effect, error)
	Return         func(ReturnContext) ([]Effect, error)
	CarriesEffects func(types.Type) bool
}

// Configure analysis budgets, frontend dispatch, and centralized type mappings.
// 配置通用编译调度、资源预算及集中类型映射。
type Options struct {
	// Declare stable JSON inputs for custom mapping or captured callback configuration without serializing function addresses.
	// 为自定义映射或回调捕获配置声明稳定的 JSON 输入，不序列化函数地址。
	Configuration map[string]json.RawMessage
	Load          LoadOptions
	Frontends     []Frontend
	MaxDepth      int
	MaxPaths      int
	MaxCalls      int
	// Maximum analyzed iterations for each synchronous repeated invocation.
	// 每个同步重复调用最多分析的迭代次数。
	MaxIterations int
	Mappers       []TypeMapper
}

// Store a writable Bundle and its compilation report.
// 保存可写入的 Bundle 和编译报告。
type Result struct {
	Bundle openapi.Bundle
	Report openapi.Report
}

// Compile real projects with registered frontends and shared annotations and projections.
// 将显式注册的前端应用于真实项目，复用注释、控制流与类型投影。
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
	if options.MaxIterations == 0 {
		options.MaxIterations = 32
	}
	if options.MaxDepth < 1 || options.MaxPaths < 1 || options.MaxCalls < 1 || options.MaxIterations < 1 {
		return nil, fmt.Errorf("openapi.analysis.budget: all budgets must be positive")
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
		// 仅在 handler 所有语句结束后提交尚未写 body 的最终状态。
		for i := range paths {
			a.finishResponse(&paths[i])
		}
		template := openapi.Template{Key: openapi.OperationKey(fn.Symbol), Symbol: fn.Symbol, Source: fn.Source, Operation: spec.Operation{Responses: map[string]spec.RefOr[spec.Response]{}}}
		if fn.Signature.Recv() == nil {
			symbol := fn.Symbol
			template.RuntimeSymbols = []string{symbol}
			if fn.Package.Name == "main" {
				// Preserve the complete symbols observed in main binaries and go test.
				// 主程序二进制与 go test 使用两种真实符号名，分别保留完整证据。
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
		declarations := project.declarations(fn, &template.Diagnostics)
		project.mergeConditionalPaths(&template, paths, data.Components.Schemas, options.Mappers)
		project.mergeDeclarations(&template, declarations, paths, data.Components.Schemas, options.Mappers)
		data.Templates = append(data.Templates, template)
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
	return &Result{Bundle: bundle, Report: openapi.Report{Diagnostics: []openapi.Diagnostic{}}}, nil
}

// Return string keys in stable order.
// 返回字符串键的稳定排序。
func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Deduplicate alternatives and use anyOf when their shapes may overlap.
// 合并有限备选 Schema；相同结果去重，重叠备选使用 anyOf。
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
// 将已传播的字面对象或真实类型转换为共同 Schema。
func (p *Project) valueSchema(v Value, direction Direction, media string, codec WireCodec, mappers []TypeMapper, components map[string]*spec.Schema) (*spec.Schema, error) {
	if v.Unknown || v.Type == nil {
		return nil, fmt.Errorf("critical payload type is unresolved")
	}
	if v.Nil || v.DynamicNil {
		return spec.Typed("null"), nil
	}
	if _, isMap := v.Type.Underlying().(*types.Map); v.Fields != nil && isMap {
		s := spec.Typed("object")
		s.Properties = map[string]*spec.Schema{}
		for _, name := range sortedKeys(v.Fields) {
			field, err := p.valueSchema(v.Fields[name], direction, media, codec, mappers, components)
			if err != nil {
				return nil, err
			}
			s.Properties[name] = field
		}
		s.Required = spec.Set(sortedKeys(v.Fields))
		return s, nil
	}
	projection, err := p.Schema(ProjectionRequest{Type: v.Type, Direction: direction, MediaType: media, Codec: codec, Mappers: mappers})
	if err != nil {
		return nil, err
	}
	for name, s := range projection.Components {
		components[name] = s
	}
	return projection.Root, nil
}

// Link neutral effects without disguising unknown responses as default.
// 将前端效果链接到共同模型；不将未知响应伪装成 default。
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
		// 状态提交和终止副作用由分析器处理，单独效果不虚构响应。
	default:
		return fmt.Errorf("unknown frontend effect %s", e.Kind)
	}
	return nil
}
