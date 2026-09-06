package compiler

import (
	"encoding/json"
	"fmt"
	"go/types"
	"strings"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/internal/comment"
	"github.com/openapi-golang/openapi/spec"
)

// Select a fully qualified Go symbol and optionally one response status.
// 选择完整 Go 符号，并可限定一个响应状态。
type ExplainQuery struct {
	Symbol   string `json:"symbol"`
	Response string `json:"response,omitempty"`
}

// Preserve parsed declarations without claiming that business code enforces them.
// 保留已解析声明，不据此声称业务代码实施了约束。
type DeclarationEvidence struct {
	Source         openapi.Source             `json:"source"`
	Description    string                     `json:"description,omitempty"`
	Values         map[string]json.RawMessage `json:"values,omitempty"`
	Implementation string                     `json:"implementation"`
}

// Associate one projected field or type with its actual Go declaration.
// 将投影后的字段或类型关联到真实 Go 声明。
type SchemaOrigin struct {
	Kind         string                `json:"kind"`
	Source       openapi.Source        `json:"source"`
	GoType       string                `json:"goType"`
	WireName     string                `json:"wireName,omitempty"`
	Required     bool                  `json:"required"`
	Schema       *spec.Schema          `json:"schema"`
	Declarations []DeclarationEvidence `json:"declarations,omitempty"`
}

// Describe the payload projection used by a neutral effect before optional response wrapping.
// 描述中立效果实际使用的载荷投影；该投影位于可选响应包装之前。
type SchemaUse struct {
	Handler          string                  `json:"handler"`
	Kind             EffectKind              `json:"kind"`
	Direction        Direction               `json:"direction"`
	Status           string                  `json:"status,omitempty"`
	MediaType        string                  `json:"mediaType,omitempty"`
	PayloadMediaType string                  `json:"payloadMediaType,omitempty"`
	In               string                  `json:"in,omitempty"`
	Name             string                  `json:"name,omitempty"`
	Source           openapi.Source          `json:"source"`
	GoType           string                  `json:"goType,omitempty"`
	Codec            string                  `json:"codec,omitempty"`
	Transformed      bool                    `json:"transformed,omitempty"`
	Provided         bool                    `json:"provided,omitempty"`
	Schema           *spec.Schema            `json:"schema,omitempty"`
	Components       map[string]*spec.Schema `json:"components,omitempty"`
	Origins          []SchemaOrigin          `json:"origins,omitempty"`
	Rules            []openapi.Source        `json:"rules,omitempty"`
	Audit            []string                `json:"audit,omitempty"`
	Error            string                  `json:"error,omitempty"`
}

// Keep the request condition attached to each selected final response.
// 保留每个最终响应对应的请求条件。
type ExplainedResponse struct {
	When     *openapi.RequestCondition `json:"when,omitempty"`
	Response spec.RefOr[spec.Response] `json:"response"`
}

// Return detached evidence, final contracts, diagnostics, and centralized remediation guidance.
// 返回独立的来源证据、最终契约、诊断及集中修复指引。
type Explanation struct {
	// Resolve final contract references using the captured Bundle components.
	// 使用捕获时的 Bundle 组件解析最终契约引用。
	Components   *spec.Components      `json:"components,omitempty"`
	Kind         string                `json:"kind"`
	Symbol       string                `json:"symbol"`
	Source       openapi.Source        `json:"source"`
	Status       string                `json:"status,omitempty"`
	Operation    *openapi.Template     `json:"operation,omitempty"`
	Responses    []ExplainedResponse   `json:"responses,omitempty"`
	Uses         []SchemaUse           `json:"uses"`
	Declarations []DeclarationEvidence `json:"declarations,omitempty"`
	Diagnostics  []openapi.Diagnostic  `json:"diagnostics,omitempty"`
	Guidance     []string              `json:"guidance"`
}

// Bound optional compile-time records separately from runtime Bundle data.
// 独立限制可选编译期记录，不写入运行时 Bundle。
type explanationCapture struct {
	bundle          openapi.Bundle
	when            *openapi.RequestCondition
	handler         string
	maxBytes, bytes int
	err             error
	uses            []SchemaUse
	known           map[string]openapi.Source
	declarations    map[string][]DeclarationEvidence
}

// Charge serialized evidence before retaining it; overflow never produces a partial successful report.
// 保存证据前计入序列化字节预算；超限不返回部分成功的报告。
func (c *explanationCapture) charge(value any) bool {
	if c.err != nil {
		return false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		c.err = fmt.Errorf("openapi.explain.encode: %w", err)
		return false
	}
	if len(raw) > c.maxBytes-c.bytes {
		c.err = fmt.Errorf("openapi.explain.budget: evidence exceeds %d bytes", c.maxBytes)
		return false
	}
	c.bytes += len(raw)
	return true
}

// Snapshot effects only when capture is requested, without invoking any frontend callback.
// 仅在显式请求时保存效果快照，不调用任何前端回调。
func (p *Project) captureProjection(site SchemaUse, projection *Projection, failure error) {
	c := p.explanations
	if c == nil || c.err != nil {
		return
	}
	site.Handler = c.handler
	if c.when != nil {
		site.Source.When = c.when
	}
	if projection != nil {
		site.Schema, site.Components = projection.Root, projection.Components
		site.Origins, site.Rules, site.Audit = projection.Origins, projection.Rules, projection.Audit
	}
	if failure != nil {
		site.Error = failure.Error()
	}
	if !c.charge(site) {
		return
	}
	detached, err := copyExplanationJSON(site)
	if err != nil {
		c.err = err
		return
	}
	c.uses = append(c.uses, detached)
}

// Describe an actual effect without retaining Go objects or executable callbacks.
// 描述实际效果，不保留 Go 对象或可执行回调。
func (p *Project) effectUse(effect Effect, direction Direction) SchemaUse {
	if p.explanations == nil {
		return SchemaUse{}
	}
	codec := ""
	media := effect.PayloadMediaType
	if media == "" {
		media = effect.MediaType
	}
	if effect.Codec != nil {
		codec = effect.Codec.Name()
	} else if effect.Payload.Type != nil && effect.WireSchema == nil && (media == "" || media == "application/json") {
		codec = "std-json"
	}
	return SchemaUse{Kind: effect.Kind, Direction: direction, Status: effect.Status, MediaType: effect.MediaType, PayloadMediaType: effect.PayloadMediaType, In: effect.In, Name: effect.Name, Source: effect.Source, GoType: explainType(effect.Payload.Type), Codec: codec, Transformed: effect.TransformSchema != nil, Provided: effect.WireSchema != nil}
}

// Use import paths instead of local aliases for stable type identities.
// 使用导入路径代替局部别名，确保类型身份稳定。
func explainType(typ types.Type) string {
	if typ == nil {
		return ""
	}
	return types.TypeString(typ, func(pkg *types.Package) string { return pkg.Path() })
}

// Freeze semantic descriptions and directive values as unproven declarations.
// 将语义描述及指令值保存为未经实施证明的声明。
func declarationEvidence(doc comment.Document, source openapi.Source) []DeclarationEvidence {
	description := strings.TrimSpace(doc.Summary + "\n\n" + doc.Description)
	if description == "" && len(doc.Directives) == 0 {
		return nil
	}
	var out []DeclarationEvidence
	for _, directive := range doc.Directives {
		origin := source
		if directive.Kind != "" {
			origin.Rule = "openapi.comment." + directive.Kind
		}
		out = append(out, DeclarationEvidence{Source: origin, Description: description, Values: directive.Values, Implementation: "not-proven"})
		description = ""
	}
	if len(out) == 0 {
		out = append(out, DeclarationEvidence{Source: source, Description: description, Implementation: "not-proven"})
	}
	return out
}

// Record a completed projection using the same metadata and codec decisions as Schema generation.
// 使用 Schema 生成时相同的元数据与 codec 决策，记录已完成的投影。
func (p *projector) recordOrigin(object types.Object, typ types.Type, schema *spec.Schema, kind, wireName string, required bool) {
	if !p.request.Explain {
		return
	}
	source := p.project.metadataSource(object)
	doc, _ := p.project.metadata(object)
	declarations := declarationEvidence(doc, source)
	source.Kind, source.Rule = "derived", "go.types."+kind
	p.origins = append(p.origins, SchemaOrigin{Kind: kind, Source: source, GoType: explainType(typ), WireName: wireName, Required: required, Schema: schema, Declarations: declarations})
}

// Retain the rule that actually handled a type, not every rule that was considered.
// 保留实际处理类型的规则，不把尝试但未采用的规则列为证据。
func (p *projector) recordRule(typ types.Type, rule, kind string) {
	if p.request.Explain {
		p.rules = append(p.rules, openapi.Source{Symbol: explainType(typ), Rule: rule, Kind: kind})
	}
}

// Index source-root fields that lack a selected projection and semantic declarations on candidate functions.
// 索引源码根中未参与投影的字段，以及候选函数的语义声明。
func (p *Project) captureDeclarations(fn Function) {
	c := p.explanations
	if c == nil {
		return
	}
	source := p.metadataSource(fn.Object)
	declarations := declarationEvidence(p.comments[fn.Object], source)
	if c.charge(declarations) {
		c.declarations[fn.Symbol] = declarations
	}
}

// Preserve known source fields without retaining the project or its AST in Result.
// 保存已知源码字段，避免 Result 持有整个项目或 AST。
func (p *Project) captureKnownFields() {
	c := p.explanations
	if c == nil {
		return
	}
	roots := map[string]bool{}
	for _, pkg := range p.Packages {
		roots[pkg.Path] = true
	}
	for object, symbol := range p.metadataSymbols {
		field, ok := object.(*types.Var)
		if !ok || !field.IsField() || field.Pkg() == nil || !roots[field.Pkg().Path()] || !strings.HasSuffix(symbol, "."+field.Name()) {
			continue
		}
		source := p.metadataSource(field)
		source.Kind, source.Rule = "declared", "go.types.field"
		c.known[symbol] = source
	}
	c.charge(c.known)
}

// Query captured evidence without reading source again or executing mappers and codecs.
// 查询已捕获的证据，不重新读取源码或执行 mapper 与 codec。
func (r *Result) Explain(query ExplainQuery) (Explanation, error) {
	if r == nil || r.explanations == nil {
		return Explanation{}, fmt.Errorf("openapi.explain.disabled: compile with Options.Explain enabled")
	}
	if strings.TrimSpace(query.Symbol) == "" {
		return Explanation{}, fmt.Errorf("openapi.explain.symbol: a fully qualified Go symbol is required")
	}
	result := Explanation{Symbol: query.Symbol, Status: query.Response, Uses: []SchemaUse{}, Guidance: []string{
		"Declarations describe contracts; enforcement is not proven. Validate actual request and response samples with contracttest.",
		"Use a centralized Frontend rule for unknown effects, WireCodec for field selection, or TypeMapper for custom wire types. Keep business handlers and DTOs unchanged.",
	}}
	c := r.explanations
	snapshot := c.bundle.Snapshot()
	handlers := map[string]bool{}
	for _, template := range snapshot.Templates {
		if template.Symbol != query.Symbol {
			continue
		}
		result.Kind, result.Source = "operation", template.Source
		result.Operation = &template
		result.Components = &snapshot.Components
		result.Source.Symbol = template.Symbol
		handlers[template.Symbol] = true
		if query.Response != "" {
			result.Kind = "response"
			result.Operation = nil
			if response, ok := template.Operation.Responses[query.Response]; ok {
				result.Responses = append(result.Responses, ExplainedResponse{Response: response})
			}
			for _, variant := range template.Variants {
				if response, ok := variant.Operation.Responses[query.Response]; ok {
					when := variant.When
					result.Responses = append(result.Responses, ExplainedResponse{When: &when, Response: response})
				}
			}
			if len(result.Responses) == 0 {
				return Explanation{}, fmt.Errorf("openapi.explain.response: no response %s for %s; inspect the handler explanation and its unresolved diagnostics", query.Response, query.Symbol)
			}
		}
		for _, declaration := range c.declarations[template.Symbol] {
			if query.Response != "" && !declarationStatus(declaration, query.Response) {
				continue
			}
			result.Declarations = append(result.Declarations, declaration)
		}
		break
	}
	if query.Response != "" && result.Kind == "" {
		return Explanation{}, fmt.Errorf("openapi.explain.response: response selection requires a handler symbol")
	}
	for _, use := range c.uses {
		if result.Kind == "operation" || result.Kind == "response" {
			if use.Handler != query.Symbol || query.Response != "" && (use.Direction != Output || use.Status != query.Response) {
				continue
			}
		} else {
			var origins []SchemaOrigin
			for _, origin := range use.Origins {
				if origin.Source.Symbol == query.Symbol {
					origins = append(origins, origin)
					result.Kind, result.Source = origin.Kind, origin.Source
				}
			}
			if len(origins) == 0 {
				continue
			}
			use.Origins = origins
			handlers[use.Handler] = true
		}
		result.Uses = append(result.Uses, use)
	}
	if result.Kind == "" {
		if source, ok := c.known[query.Symbol]; ok {
			result.Kind, result.Source = "field", source
			result.Guidance = append(result.Guidance, "The field exists in the loaded source but has no selected payload projection. Inspect the handler diagnostics, build selection, and codec field selection; no wire behavior is inferred.")
		} else {
			return Explanation{}, fmt.Errorf("openapi.explain.symbol: no captured origin for %s; select a loaded handler or projected type/field, or inspect codec and build selection", query.Symbol)
		}
	}
	for _, template := range snapshot.Templates {
		if !handlers[template.Symbol] {
			continue
		}
		result.Diagnostics = append(result.Diagnostics, template.Diagnostics...)
		for _, variant := range template.Variants {
			result.Diagnostics = append(result.Diagnostics, variant.Diagnostics...)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Fix != "" {
			result.Guidance = append(result.Guidance, diagnostic.Fix)
		}
	}
	return copyExplanationJSON(result)
}

// Match parsed numeric or string statuses without changing their original JSON values.
// 匹配已解析的数字或字符串状态，不改写原始 JSON 值。
func declarationStatus(declaration DeclarationEvidence, status string) bool {
	if declaration.Source.Rule != "openapi.comment.response" {
		return false
	}
	raw := declaration.Values["status"]
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value == status
	}
	return string(raw) == status
}

// Detach nested schemas, raw JSON, maps, slices, and conditions in one checked copy.
// 通过一次经过检查的复制隔离嵌套 Schema、原始 JSON、映射、切片及条件。
func copyExplanationJSON[T any](value T) (T, error) {
	var out T
	raw, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	if err != nil {
		return out, fmt.Errorf("openapi.explain.encode: %w", err)
	}
	return out, nil
}

// Scope captured evidence to the current path while leaving generated facts unchanged.
// 将捕获证据限定于当前路径，不改变生成的事实数据。
func (p *Project) explanationPath(when openapi.RequestCondition) func() {
	if p.explanations == nil {
		return func() {}
	}
	previous := p.explanations.when
	p.explanations.when = nil
	if !when.Unconditional() {
		p.explanations.when = &when
	}
	return func() { p.explanations.when = previous }
}
