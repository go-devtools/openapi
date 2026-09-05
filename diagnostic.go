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

// 只进行离线结构与语义检查，绝不抓取外部引用。
func Check(data []byte) Report {
	r := Report{Diagnostics: []Diagnostic{}}
	for _, i := range validate.Check(data) {
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
