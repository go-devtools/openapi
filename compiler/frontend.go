package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// 表示有限传播的静态值；未知值从不伪装成具体 payload。
// Represent a propagated static value without inventing unknown payloads.
type Value struct {
	Type     types.Type
	Constant constant.Value
	Fields   map[string]Value
	Nil      bool
	Unknown  bool
}

// 表示框架前端输出的中立效果。
// Represent a framework-neutral frontend effect.
type EffectKind string

// 共同效果描述网络事实，不包含任何框架方法名。
// Describe wire facts without framework method names.
const (
	RequestBody    EffectKind = "requestBody"
	ParameterRead  EffectKind = "parameter"
	ResponseBody   EffectKind = "responseBody"
	ResponseStatus EffectKind = "status"
	ResponseCommit EffectKind = "commit"
	Handled        EffectKind = "handled"
	ResponseHeader EffectKind = "header"
	Abort          EffectKind = "abort"
	Unresolved     EffectKind = "unresolved"
)

// 记录请求、响应、状态和控制效果及其来源。
// Record request, response, status, and control effects with their sources.
type Effect struct {
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

// 提供标准库调用视图和核心已传播的实参，不暴露第三方 SSA。
// Expose standard-library call views and propagated arguments without third-party SSA.
type CallContext struct {
	Function  Function
	Call      *ast.CallExpr
	Object    *types.Func
	Arguments []Value
	Receiver  Value
	Source    openapi.Source
}

// 支持返回响应值或 error 的前端形态。
// Support frontends whose handlers return responses or errors.
type ReturnContext struct {
	Function Function
	Values   []Value
	Source   openapi.Source
}

// 显式注册前端规则；未提供的回调表示此类入口没有框架规则。
// Register frontend rules explicitly; absent callbacks define no rules.
type Frontend struct {
	Name           string
	Match          func(Function) bool
	Entry          func(Function) []Effect
	Call           func(CallContext) ([]Effect, error)
	Return         func(ReturnContext) ([]Effect, error)
	CarriesEffects func(types.Type) bool
}

// 配置通用编译调度、资源预算及集中类型映射。
// Configure analysis budgets, frontend dispatch, and centralized type mappings.
type Options struct {
	Load      LoadOptions
	Frontends []Frontend
	MaxDepth  int
	MaxPaths  int
	MaxCalls  int
	Mappers   []TypeMapper
}

// 保存可写入的 Bundle 和编译报告。
// Store a writable Bundle and its compilation report.
type Result struct {
	Bundle openapi.Bundle
	Report openapi.Report
}

// 将显式注册的前端应用于真实项目，复用注释、控制流与类型投影。
// Compile real projects with registered frontends and shared annotations and projections.
func Compile(ctx context.Context, options Options) (*Result, error) {
	if len(options.Frontends) == 0 {
		return nil, fmt.Errorf("openapi.frontend.missing: 至少显式注册一个前端")
	}
	names := map[string]bool{}
	for _, f := range options.Frontends {
		if f.Name == "" || f.Match == nil || names[f.Name] {
			return nil, fmt.Errorf("openapi.frontend.invalid: 名称或候选规则不合法")
		}
		names[f.Name] = true
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
	if options.MaxDepth < 1 || options.MaxPaths < 1 || options.MaxCalls < 1 {
		return nil, fmt.Errorf("openapi.analysis.budget: 所有预算必须为正数")
	}
	fingerprint, err := project.fingerprint(options)
	if err != nil {
		return nil, err
	}
	data := openapi.BundleData{FormatVersion: openapi.BundleFormatVersion, SpecVersion: "3.2.0", Capabilities: []string{"oas32", "schema2020-12"}, Fingerprint: fingerprint, Components: spec.Components{Schemas: map[string]*spec.Schema{}}, Profile: openapi.BuildProfile{GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Generator: "openapi/compiler", Codec: "explicit-profile"}}
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
			return nil, fmt.Errorf("openapi.frontend.ambiguous: %s 匹配多个前端", fn.Symbol)
		}
		front := matched[0]
		a := analyzer{ctx: ctx, project: project, options: options, frontend: front}
		initial := flow{values: map[types.Object]Value{}}
		for i := 0; i < fn.Signature.Params().Len(); i++ {
			param := fn.Signature.Params().At(i)
			initial.values[param] = Value{Type: param.Type()}
		}
		if front.Entry != nil {
			a.effects(&initial, front.Entry(fn))
		}
		var paths []flow
		if fn.Declaration.Body == nil {
			a.unknown(&initial, fn.Source, "候选没有可分析函数体")
			paths = []flow{initial}
		} else {
			paths = a.statements(fn, fn.Declaration.Body.List, []flow{initial}, 0, true)
		}
		// 仅在 handler 所有语句结束后提交尚未写 body 的最终状态。
		// Finalize a pending bodyless status only after all handler statements finish.
		for i := range paths {
			if paths[i].writes == 0 && paths[i].pending != nil {
				paths[i].effects = append(paths[i].effects, *paths[i].pending)
			}
		}
		template := openapi.Template{Key: openapi.OperationKey(fn.Symbol), Symbol: fn.Symbol, Source: fn.Source, Operation: spec.Operation{Responses: map[string]spec.RefOr[spec.Response]{}}}
		if fn.Signature.Recv() == nil {
			symbol := fn.Symbol
			template.RuntimeSymbols = []string{symbol}
			if fn.Package.Name == "main" {
				// 主程序二进制与 go test 使用两种真实符号名，分别保留完整证据。
				// Preserve the complete symbols observed in main binaries and go test.
				template.RuntimeSymbols = append(template.RuntimeSymbols, "main."+fn.Object.Name())
			}
		}
		doc := project.comments[fn.Object]
		template.Operation.Summary = doc.Summary
		template.Operation.Description = doc.Description
		for _, d := range doc.Directives {
			if d.Kind != "" {
				template.Diagnostics = append(template.Diagnostics, openapi.Diagnostic{Code: "openapi.declaration.unresolved", Severity: openapi.Error, Message: "请求响应兜底声明尚未解析", Fix: "使用已注册前端事实或集中规则", Source: fn.Source})
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
					err = fmt.Errorf("函数注释不接受 %s", k)
				}
				if err != nil {
					return nil, fmt.Errorf("openapi.comment.context: %w", err)
				}
			}
		}
		template.Diagnostics = append(template.Diagnostics, a.diagnostics...)
		for _, path := range paths {
			template.Diagnostics = append(template.Diagnostics, path.diagnostics...)
			for _, effect := range path.effects {
				template.Facts = append(template.Facts, effect.Source)
				if err := project.mergeEffect(&template.Operation, effect, data.Components.Schemas, options.Mappers); err != nil {
					template.Diagnostics = append(template.Diagnostics, openapi.Diagnostic{Code: "openapi.effect.unresolved", Severity: openapi.Error, Message: err.Error(), Fix: "注册集中前端规则或 TypeMapper", Source: effect.Source})
				}
			}
		}
		data.Templates = append(data.Templates, template)
	}
	data.Profile.Frontend = strings.Join(sortedKeys(names), ",")
	bundle, err := openapi.NewBundle(data)
	if err != nil {
		return nil, err
	}
	return &Result{Bundle: bundle, Report: openapi.Report{Diagnostics: []openapi.Diagnostic{}}}, nil
}

// 返回字符串键的稳定排序。
// Return string keys in stable order.
func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// 从源码字节、有效依赖源码和前端预算计算可复现指纹。
// Fingerprint source bytes, effective dependencies, and frontend budgets.
func (p *Project) fingerprint(options Options) (string, error) {
	hash := sha256.New()
	files := append([]string(nil), p.sourceFiles...)
	sort.Strings(files)
	// 哈希条目按内容摘要排序，避免绝对目录变化改变指纹。
	// Sort content entries so absolute checkout paths do not affect fingerprints.
	var entries []string
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		if filepath.Base(file) == "zz_openapi.gen.go" && strings.HasPrefix(string(raw), "// Code generated by openapi/compiler.") {
			continue
		}
		sum := sha256.Sum256(raw)
		entries = append(entries, filepath.Base(file)+":"+hex.EncodeToString(sum[:]))
	}
	sort.Strings(entries)
	for _, entry := range entries {
		fmt.Fprintln(hash, entry)
	}
	var fronts []string
	for _, f := range options.Frontends {
		fronts = append(fronts, f.Name)
	}
	sort.Strings(fronts)
	fmt.Fprintf(hash, "%s|%s|%s|%v|%v|%d|%d|%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, fronts, options.Load.BuildFlags, options.MaxDepth, options.MaxPaths, options.MaxCalls)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// 合并有限备选 Schema；相同结果去重，重叠备选使用 anyOf。
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

// 将已传播的字面对象或真实类型转换为共同 Schema。
// Project propagated literals or real types into shared Schemas.
func (p *Project) valueSchema(v Value, direction Direction, media string, codec WireCodec, mappers []TypeMapper, components map[string]*spec.Schema) (*spec.Schema, error) {
	if v.Unknown || v.Type == nil {
		return nil, fmt.Errorf("关键 payload 类型未解决")
	}
	if v.Nil {
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

// 将前端效果链接到共同模型；不将未知响应伪装成 default。
// Link neutral effects without disguising unknown responses as default.
func (p *Project) mergeEffect(op *spec.Operation, e Effect, components map[string]*spec.Schema, mappers []TypeMapper) error {
	switch e.Kind {
	case Unresolved:
		return fmt.Errorf("%s；%s", e.Message, e.Fix)
	case RequestBody:
		schema, err := p.valueSchema(e.Payload, Input, e.MediaType, e.Codec, mappers, components)
		if err != nil {
			return err
		}
		if op.RequestBody == nil {
			body := spec.Inline(spec.RequestBody{Content: map[string]spec.RefOr[spec.MediaType]{}})
			op.RequestBody = &body
		}
		body := op.RequestBody.Value
		body.Required = body.Required || e.Required
		media := body.Content[e.MediaType]
		if media.Value == nil {
			media = spec.Inline(spec.MediaType{})
		}
		media.Value.Schema = union(media.Value.Schema, schema)
		body.Content[e.MediaType] = media
	case ParameterRead:
		if e.Name == "" {
			return fmt.Errorf("参数名称不是可求值常量")
		}
		for _, existing := range op.Parameters {
			if existing.Value != nil && existing.Value.Name == e.Name && existing.Value.In == e.In {
				return nil
			}
		}
		schema, err := p.valueSchema(e.Payload, Input, "application/json", e.Codec, mappers, components)
		if err != nil {
			return err
		}
		op.Parameters = append(op.Parameters, spec.Inline(spec.Parameter{Name: e.Name, In: e.In, Required: e.Required || e.In == "path", Schema: schema}))
	case ResponseBody, ResponseStatus:
		if e.Status == "" {
			return fmt.Errorf("响应状态不是可求值常量")
		}
		response := op.Responses[e.Status]
		if response.Value == nil {
			response = spec.Inline(spec.Response{Description: "响应 " + e.Status, Content: map[string]spec.RefOr[spec.MediaType]{}})
		}
		if e.Kind == ResponseBody {
			if e.MediaType == "" {
				return fmt.Errorf("响应媒体类型未解决")
			}
			schema, err := p.valueSchema(e.Payload, Output, e.MediaType, e.Codec, mappers, components)
			if err != nil {
				return err
			}
			media := response.Value.Content[e.MediaType]
			if media.Value == nil {
				media = spec.Inline(spec.MediaType{})
			}
			media.Value.Schema = union(media.Value.Schema, schema)
			response.Value.Content[e.MediaType] = media
		}
		op.Responses[e.Status] = response
	case ResponseHeader, Abort:
		// 状态提交和终止副作用由分析器处理，单独效果不虚构响应。
		// The analyzer handles commits and termination without inventing responses.
	default:
		return fmt.Errorf("未知前端效果 %s", e.Kind)
	}
	return nil
}
