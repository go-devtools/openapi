package contracttest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// 限制测试样本的总字节、单行字节及条目数，避免无界流占用测试进程。
type Limits struct {
	MaxBytes     int64
	MaxLineBytes int
	MaxItems     int
}

// 填充明确的默认预算并拒绝非法配置。
func (l Limits) normalized() (Limits, error) {
	if l.MaxBytes == 0 {
		l.MaxBytes = 8 << 20
	}
	if l.MaxLineBytes == 0 {
		l.MaxLineBytes = 1 << 20
	}
	if l.MaxItems == 0 {
		l.MaxItems = 10000
	}
	if l.MaxBytes < 1 || l.MaxLineBytes < 1 || l.MaxItems < 1 {
		return l, fmt.Errorf("openapi.contract.budget: 预算必须为正数")
	}
	return l, nil
}

// 执行有预算的行扫描，允许 CR、LF 和 CRLF；回调不接收行尾。
func lines(reader io.Reader, limits Limits, visit func(string) error) error {
	if reader == nil {
		return fmt.Errorf("openapi.contract.reader: 缺少输入流")
	}
	limited := &io.LimitedReader{R: reader, N: limits.MaxBytes + 1}
	scan := bufio.NewScanner(limited)
	scan.Buffer(make([]byte, min(4096, limits.MaxLineBytes+2)), limits.MaxLineBytes+2)
	scan.Split(splitLine)
	for scan.Scan() {
		if limited.N == 0 {
			return fmt.Errorf("openapi.contract.budget: 流超过字节预算")
		}
		line := scan.Text()
		if len(line) > limits.MaxLineBytes {
			return fmt.Errorf("openapi.contract.budget: 行超过字节预算")
		}
		if err := visit(line); err != nil {
			return err
		}
	}
	if err := scan.Err(); err != nil {
		return fmt.Errorf("openapi.contract.stream: %w", err)
	}
	if limited.N == 0 {
		return fmt.Errorf("openapi.contract.budget: 流超过字节预算")
	}
	return nil
}

// 遵循 SSE 的三种换行表示，并在 CR 后等待可能的 LF。
func splitLine(data []byte, atEOF bool) (int, []byte, error) {
	for i, c := range data {
		if c == '\n' {
			return i + 1, data[:i], nil
		}
		if c == '\r' {
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			n := 1
			if i+1 < len(data) && data[i+1] == '\n' {
				n = 2
			}
			return i + n, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// 对每行 JSON 独立应用 itemSchema；空行明确忽略。
func (v *Validator) NDJSON(reader io.Reader, limits Limits) error {
	l, err := limits.normalized()
	if err != nil {
		return err
	}
	count := 0
	return lines(reader, l, func(line string) error {
		if strings.TrimSpace(line) == "" {
			return nil
		}
		count++
		if count > l.MaxItems {
			return fmt.Errorf("openapi.contract.budget: 流条目超过预算")
		}
		if err := v.JSON([]byte(line)); err != nil {
			return fmt.Errorf("条目 %d: %w", count, err)
		}
		return nil
	})
}

// 将真正 SSE 帧转换为 OAS 三点二定义的事件对象，data 保持字符串。
func ParseSSE(reader io.Reader, limits Limits) ([]map[string]any, error) {
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	events := []map[string]any{}
	fields := map[string]any{}
	var data []string
	first := true
	err = lines(reader, l, func(line string) error {
		if first {
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		// 网络 SSE 采用 UTF-8；替换无效字节以符合文本解码语义。
		if !utf8.ValidString(line) {
			line = string(bytes.ToValidUTF8([]byte(line), []byte("�")))
		}
		if line == "" {
			if len(data) > 0 {
				fields["data"] = strings.Join(data, "\n")
				events = append(events, fields)
				if len(events) > l.MaxItems {
					return fmt.Errorf("openapi.contract.budget: 事件数超过预算")
				}
			}
			fields = map[string]any{}
			data = nil
			return nil
		}
		if strings.HasPrefix(line, ":") {
			return nil
		}
		name, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch name {
		case "data":
			data = append(data, value)
		case "event":
			fields[name] = value
		case "id":
			if !strings.ContainsRune(value, 0) {
				fields[name] = value
			}
		case "retry":
			valid := value != ""
			for _, c := range value {
				if c < '0' || c > '9' {
					valid = false
				}
			}
			if valid {
				digits := strings.TrimLeft(value, "0")
				if digits == "" {
					digits = "0"
				}
				fields[name] = json.Number(digits)
			}
		}
		return nil
	})
	// 未以空行结束的帧不构成已分派事件。
	if err != nil {
		return nil, err
	}
	return events, nil
}

// 对已按 SSE 协议解析的每个事件应用 itemSchema。
func (v *Validator) SSE(reader io.Reader, limits Limits) error {
	events, err := ParseSSE(reader, limits)
	if err != nil {
		return err
	}
	for i, event := range events {
		if err = v.Value(event); err != nil {
			return fmt.Errorf("事件 %d: %w", i+1, err)
		}
	}
	return nil
}
