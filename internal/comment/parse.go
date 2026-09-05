// 解析普通语义说明与单一 @openapi 指令语法。
// Parse ordinary documentation and the shared @openapi directive syntax.
package comment

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// 限制单条注释的资源占用。
// Bound the size of each comment.
const MaxBytes = 65536

// 保存语义说明及有来源位置的指令。
// Store semantic documentation and located directives.
type Document struct {
	Summary     string
	Description string
	Directives  []Directive
}

// 保存一个指令行；空 Kind 表示普通约束。
// Represent one directive; an empty kind means ordinary constraints.
type Directive struct {
	Kind   string
	Values map[string]json.RawMessage
	Line   int
	Column int
}

// 提供可映射为公开诊断的稳定错误位置。
// Expose stable parse locations for public diagnostics.
type Error struct {
	Code    string
	Line    int
	Column  int
	Message string
}

// 输出适合命令行阅读的错误信息。
// Format errors for command-line readers.
func (e *Error) Error() string {
	return fmt.Sprintf("%s %d:%d %s", e.Code, e.Line, e.Column, e.Message)
}

// 枚举统一语法允许的名称；具体上下文适用性由投影阶段检查。
// List recognized directive names; projection checks contextual applicability.
var allowed = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range strings.Fields("required nonnull nullable operationId tags deprecated ignore enum const default examples format pattern minLength maxLength minimum maximum exclusiveMinimum exclusiveMaximum multipleOf minItems maxItems uniqueItems minContains maxContains minProperties maxProperties readOnly writeOnly contentEncoding contentMediaType title description mediaType type status") {
		m[k] = true
	}
	return m
}()

// 扫描真实 JSON 值，不按空格切分嵌套对象、数组或字符串。
// Scan real JSON values without splitting nested content on whitespace.
func Parse(src string) (Document, error) {
	var out Document
	if len(src) > MaxBytes {
		return out, &Error{Code: "openapi.comment.budget", Line: 1, Column: 1, Message: "注释超过字节预算"}
	}
	var prose []string
	assigned := map[string]string{}
	for idx, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "/*"))
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		if !strings.HasPrefix(line, "@openapi") {
			prose = append(prose, line)
			continue
		}
		fail := func(code string, col int, msg string) error {
			return &Error{Code: code, Line: idx + 1, Column: col + 1, Message: msg}
		}
		if len(line) > 8 && !unicode.IsSpace(rune(line[8])) {
			return out, fail("openapi.comment.syntax", 8, "指令前缀后缺少空白")
		}
		d := Directive{Values: map[string]json.RawMessage{}, Line: idx + 1, Column: 1}
		pos := 8
		for pos < len(line) {
			for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
				pos++
			}
			if pos == len(line) {
				break
			}
			start := pos
			for pos < len(line) && ((line[pos] >= 'a' && line[pos] <= 'z') || (line[pos] >= 'A' && line[pos] <= 'Z')) {
				pos++
			}
			key := line[start:pos]
			if key == "" {
				return out, fail("openapi.comment.syntax", pos, "需要指令名称")
			}
			if (key == "request" || key == "response") && len(d.Values) == 0 && d.Kind == "" {
				d.Kind = key
				continue
			}
			if !allowed[key] {
				return out, fail("openapi.comment.unknown", start, "未知指令："+key)
			}
			value := json.RawMessage("true")
			if pos < len(line) && line[pos] == '=' {
				pos++
				dec := json.NewDecoder(strings.NewReader(line[pos:]))
				dec.UseNumber()
				if err := dec.Decode(&value); err != nil {
					return out, fail("openapi.comment.json", pos, "非法 JSON 值："+err.Error())
				}
				pos += int(dec.InputOffset())
			}
			if pos < len(line) && line[pos] != ' ' && line[pos] != '\t' {
				return out, fail("openapi.comment.syntax", pos, "指令之间需要空白")
			}
			if _, ok := d.Values[key]; ok {
				return out, fail("openapi.comment.conflict", start, "同一指令重复赋值："+key)
			}
			if d.Kind == "" {
				if old, ok := assigned[key]; ok && old != string(value) {
					return out, fail("openapi.comment.conflict", start, "跨行指令冲突："+key)
				}
				assigned[key] = string(value)
			}
			d.Values[key] = append(json.RawMessage(nil), value...)
		}
		if len(d.Values) == 0 {
			return out, fail("openapi.comment.syntax", 0, "空指令")
		}
		out.Directives = append(out.Directives, d)
	}
	plain := strings.TrimSpace(strings.Join(prose, "\n"))
	if plain != "" {
		parts := strings.SplitN(plain, "\n", 2)
		out.Summary = parts[0]
		if len(parts) == 2 {
			out.Description = strings.TrimSpace(parts[1])
		}
	}
	return out, nil
}
