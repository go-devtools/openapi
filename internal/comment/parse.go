// Parse ordinary documentation and the shared @openapi directive syntax.
// 解析普通语义说明与单一 @openapi 指令语法。
package comment

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Bound the size of each comment.
// 限制单条注释的资源占用。
const MaxBytes = 65536

// Store semantic documentation and located directives.
// 保存语义说明及有来源位置的指令。
type Document struct {
	Summary     string
	Description string
	Directives  []Directive
}

// Represent one directive; an empty kind means ordinary constraints.
// 保存一个指令行；空 Kind 表示普通约束。
type Directive struct {
	Kind   string
	Values map[string]json.RawMessage
	Line   int
	Column int
}

// Expose stable parse locations for public diagnostics.
// 提供可映射为公开诊断的稳定错误位置。
type Error struct {
	Code    string
	Line    int
	Column  int
	Message string
}

// Format errors for command-line readers.
// 输出适合命令行阅读的错误信息。
func (e *Error) Error() string {
	return fmt.Sprintf("%s %d:%d %s", e.Code, e.Line, e.Column, e.Message)
}

// List recognized directive names; projection checks contextual applicability.
// 枚举统一语法允许的名称；具体上下文适用性由投影阶段检查。
var allowed = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range strings.Fields("required nonnull nullable operationId tags deprecated ignore enum const default examples format pattern minLength maxLength minimum maximum exclusiveMinimum exclusiveMaximum multipleOf minItems maxItems uniqueItems minContains maxContains minProperties maxProperties readOnly writeOnly contentEncoding contentMediaType title description mediaType type status") {
		m[k] = true
	}
	return m
}()

// Scan real JSON values without splitting nested content on whitespace.
// 扫描真实 JSON 值，不按空格切分嵌套对象、数组或字符串。
func Parse(src string) (Document, error) {
	var out Document
	if len(src) > MaxBytes {
		return out, &Error{Code: "openapi.comment.budget", Line: 1, Column: 1, Message: "comments exceed the byte budget"}
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
			return out, fail("openapi.comment.syntax", 8, "whitespace is required after the directive prefix")
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
				return out, fail("openapi.comment.syntax", pos, "directive name is required")
			}
			if (key == "request" || key == "response") && len(d.Values) == 0 && d.Kind == "" {
				d.Kind = key
				continue
			}
			if !allowed[key] {
				return out, fail("openapi.comment.unknown", start, "unknown directive: "+key)
			}
			value := json.RawMessage("true")
			if pos < len(line) && line[pos] == '=' {
				pos++
				dec := json.NewDecoder(strings.NewReader(line[pos:]))
				dec.UseNumber()
				if err := dec.Decode(&value); err != nil {
					return out, fail("openapi.comment.json", pos, "invalid JSON value: "+err.Error())
				}
				pos += int(dec.InputOffset())
			}
			if pos < len(line) && line[pos] != ' ' && line[pos] != '\t' {
				return out, fail("openapi.comment.syntax", pos, "whitespace is required between directives")
			}
			if _, ok := d.Values[key]; ok {
				return out, fail("openapi.comment.conflict", start, "duplicate assignment in the same directive: "+key)
			}
			if d.Kind == "" {
				if old, ok := assigned[key]; ok && old != string(value) {
					return out, fail("openapi.comment.conflict", start, "conflicting directives across lines: "+key)
				}
				assigned[key] = string(value)
			}
			d.Values[key] = append(json.RawMessage(nil), value...)
		}
		if len(d.Values) == 0 {
			return out, fail("openapi.comment.syntax", 0, "empty directive")
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
