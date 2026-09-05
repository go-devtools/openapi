// 编译与链接 Go 契约的框架中立运行时。
package openapi

import (
	"encoding/json"
	"fmt"
	"github.com/openapi-golang/openapi/internal/validate"
)

// 表示诊断严重程度。
type Severity string

// 错误阻止文档构建，警告与说明保留于审计报告。
const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// 使用相对文件位置记录来源，不携带机器专属路径。
type Source struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Symbol string `json:"symbol,omitempty"`
	Rule   string `json:"rule,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

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

// 保存来源事实与诊断，返回时总是防御性复制。
type Report struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Facts       []Source     `json:"facts,omitempty"`
}

// 判断报告中是否存在阻断错误。
func (r Report) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// 作为 error 返回稳定摘要，完整细节仍通过 Diagnostics 读取。
func (r Report) Error() string {
	for _, d := range r.Diagnostics {
		if d.Severity == Error {
			return fmt.Sprintf("%s: %s；%s", d.Code, d.Message, d.Fix)
		}
	}
	return "无阻断错误"
}

// 配置离线检查的基准 URI、显式资源和总预算；输入仅在调用期间只读使用。
type CheckOptions struct {
	// 指定主文档的绝对检索 URI；省略时使用本次检查的内部默认地址。
	BaseURI string
	// 预载完整 OpenAPI 或 JSON Schema 文档，键为无片段的绝对检索 URI。
	Resources map[string][]byte
	// 预载 externalValue 的原始示例字节，不把它们解释为 Schema。
	ExampleResources map[string][]byte
	// 限制主文档和所有预载内容的总字节数；零值采用八 MiB。
	MaxBytes int
	// 限制主文档、预载条目及内嵌 $id 资源的总数；零值采用六十四项。
	MaxResources int
	// 限制规范中的引用总次数；零值采用一万次。
	MaxReferences int
	// 限制索引路径、URI 解析和诊断的累计文本处理量；零值采用十六 MiB。
	// Limits cumulative index paths, URI resolution, and diagnostic text; zero uses sixteen MiB.
	MaxIndexBytes int
}

// 接收显式离线配置；验证期间不会自动获取资源。
func CheckWithOptions(data []byte, options CheckOptions) Report {
	return issuesReport(validate.CheckWithOptions(data, options.internal()))
}

// 将公开预算配置转换为内部检查器输入，不复制或修改调用方内容。
func (o CheckOptions) internal() validate.Options {
	return validate.Options{BaseURI: o.BaseURI, Resources: o.Resources, ExampleResources: o.ExampleResources, MaxBytes: o.MaxBytes, MaxResources: o.MaxResources, MaxReferences: o.MaxReferences, MaxIndexBytes: o.MaxIndexBytes}
}

// 只进行离线结构与语义检查，绝不抓取外部引用。
func Check(data []byte) Report {
	return CheckWithOptions(data, CheckOptions{})
}

// 将内部规范诊断转换为公开报告，保留全部错误位置。
func issuesReport(issues []validate.Issue) Report {
	r := Report{Diagnostics: []Diagnostic{}}
	for _, i := range issues {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: i.Code, Severity: Error, Message: i.Path + ": " + i.Message, Fix: i.Fix})
	}
	return r
}

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
