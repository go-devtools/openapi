// Provide the framework-neutral runtime for compiled Go contracts.
// 编译与链接 Go 契约的框架中立运行时。
package openapi

import (
	"encoding/json"
	"fmt"
	"github.com/openapi-golang/openapi/internal/validate"
)

// Identify diagnostic severity.
// 表示诊断严重程度。
type Severity string

// Errors block construction; warnings and notes remain in the audit report.
// 错误阻止文档构建，警告与说明保留于审计报告。
const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// Record relative source locations without machine-specific paths.
// 使用相对文件位置记录来源，不携带机器专属路径。
type Source struct {
	// Preserve the finite request condition under which this fact applies.
	// 保留此事实生效的有限请求条件。
	When   *RequestCondition `json:"when,omitempty"`
	File   string            `json:"file,omitempty"`
	Line   int               `json:"line,omitempty"`
	Column int               `json:"column,omitempty"`
	Symbol string            `json:"symbol,omitempty"`
	Rule   string            `json:"rule,omitempty"`
	Kind   string            `json:"kind,omitempty"`
}

// Describe a stable diagnostic with an explanation and remediation.
// 表示可解释、可修复且稳定编码的诊断。
type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Fix      string   `json:"fix,omitempty"`
	Source   Source   `json:"source,omitempty"`
	Route    string   `json:"route,omitempty"`
	Facts    []Source `json:"facts,omitempty"`
}

// Store facts and diagnostics with defensive-copy access.
// 保存来源事实与诊断，返回时总是防御性复制。
type Report struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Facts       []Source     `json:"facts,omitempty"`
}

// Report whether any diagnostic blocks construction.
// 判断报告中是否存在阻断错误。
func (r Report) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Return a stable error summary while retaining full diagnostic details.
// 作为 error 返回稳定摘要，完整细节仍通过 Diagnostics 读取。
func (r Report) Error() string {
	for _, d := range r.Diagnostics {
		if d.Severity == Error {
			return fmt.Sprintf("%s: %s; %s", d.Code, d.Message, d.Fix)
		}
	}
	return "No blocking errors"
}

// Configure offline base URIs, explicit resources, and aggregate budgets; inputs remain read-only during the call.
// 配置离线检查的基准 URI、显式资源和总预算；输入仅在调用期间只读使用。
type CheckOptions struct {
	// Set the root document's absolute retrieval URI or use the internal default for this check.
	// 指定主文档的绝对检索 URI；省略时使用本次检查的内部默认地址。
	BaseURI string
	// Preload complete OpenAPI or JSON Schema documents keyed by absolute, fragment-free retrieval URIs.
	// 预载完整 OpenAPI 或 JSON Schema 文档，键为无片段的绝对检索 URI。
	Resources map[string][]byte
	// Preload raw externalValue example bytes without interpreting them as schemas.
	// 预载 externalValue 的原始示例字节，不把它们解释为 Schema。
	ExampleResources map[string][]byte
	// Limit aggregate bytes across the root and all preloaded contents; zero selects eight MiB.
	// 限制主文档和所有预载内容的总字节数；零值采用八 MiB。
	MaxBytes int
	// Limit root documents, preloaded entries, and embedded $id resources; zero selects sixty-four.
	// 限制主文档、预载条目及内嵌 $id 资源的总数；零值采用六十四项。
	MaxResources int
	// Limit reference occurrences across the specification; zero selects ten thousand.
	// 限制规范中的引用总次数；零值采用一万次。
	MaxReferences int
	// Limits cumulative index paths, URI resolution, and diagnostic text; zero uses sixteen MiB.
	// 限制索引路径、URI 解析和诊断的累计文本处理量；零值采用十六 MiB。
	MaxIndexBytes int
}

// Accept explicit offline options without automatically retrieving resources during validation.
// 接收显式离线配置；验证期间不会自动获取资源。
func CheckWithOptions(data []byte, options CheckOptions) Report {
	return issuesReport(validate.CheckWithOptions(data, options.internal()))
}

// Translate public budgets into internal options without copying or modifying caller-owned contents.
// 将公开预算配置转换为内部检查器输入，不复制或修改调用方内容。
func (o CheckOptions) internal() validate.Options {
	return validate.Options{BaseURI: o.BaseURI, Resources: o.Resources, ExampleResources: o.ExampleResources, MaxBytes: o.MaxBytes, MaxResources: o.MaxResources, MaxReferences: o.MaxReferences, MaxIndexBytes: o.MaxIndexBytes}
}

// Check structure and semantics offline without fetching references.
// 只进行离线结构与语义检查，绝不抓取外部引用。
func Check(data []byte) Report {
	return CheckWithOptions(data, CheckOptions{})
}

// Convert internal diagnostics into public reports while preserving every error location.
// 将内部规范诊断转换为公开报告，保留全部错误位置。
func issuesReport(issues []validate.Issue) Report {
	r := Report{Diagnostics: []Diagnostic{}}
	for _, i := range issues {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: i.Code, Severity: Error, Message: i.Path + ": " + i.Message, Fix: i.Fix})
	}
	return r
}

// Copy serializable public data to isolate mutable collections.
// 复制可序列化公开数据，防止共享集合被外部修改。
func copyJSON[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out T
	if json.Unmarshal(b, &out) != nil {
		return v
	}
	return out
}
