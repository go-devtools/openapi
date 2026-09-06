package contracttest_test

import (
	"strings"
	"testing"
	"testing/iotest"

	ct "github.com/openapi-golang/openapi/contracttest"
)

// Replace malformed UTF-8 subparts independently; one-byte reads must preserve BOM, line endings, and data joining.
// UTF-8 错误子序列分别替换；单字节读取不能改变 BOM、换行或数据拼接。
func TestSSEUTF8AndChunking(t *testing.T) {
	for _, sample := range []struct{ name, input, want string }{
		{"adjacent", "data:\xff\xfe\n\n", "��"},
		{"partial", "data:\xe1\x80\n\n", "�"},
		{"overlong", "data:\xe0\x80\x80\n\n", "���"},
		{"surrogate", "data:\xed\xa0\x80\n\n", "���"},
		{"beyond", "data:\xf4\x90\x80\x80\n\n", "����"},
		{"interrupted", "data:\xe1\x80A\n\n", "�A"},
		{"bom-crlf", "\ufeffdata:a\r\ndata:b\r\n\r\n", "a\nb"},
		{"bare-cr", "data:a\rdata:b\r\r", "a\nb"},
		{"inner-bom", "data:\ufeff\n\n", "\ufeff"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			events, err := ct.ParseSSE(iotest.OneByteReader(strings.NewReader(sample.input)), ct.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0]["data"] != sample.want {
				t.Fatalf("wrong decoded event: %#v, want %q", events, sample.want)
			}
		})
	}
}

// Extreme budgets return errors instead of overflowing or panicking, while exact byte and line bounds remain usable.
// 极限预算返回错误而非溢出或 panic，精确字节及行边界仍然可用。
func TestStreamBudgetBoundaries(t *testing.T) {
	for _, limits := range []ct.Limits{{MaxBytes: 1<<63 - 1}, {MaxLineBytes: int(^uint(0) >> 1)}} {
		func() {
			defer func() {
				if failure := recover(); failure != nil {
					t.Errorf("budget configuration panicked: %v", failure)
				}
			}()
			if _, err := ct.ParseSSE(strings.NewReader("data:x\n\n"), limits); err == nil {
				t.Error("overflowing budget was accepted")
			}
		}()
	}
	for _, ending := range []string{"\n", "\r", "\r\n"} {
		input := "data:x" + ending + ending
		events, err := ct.ParseSSE(iotest.OneByteReader(strings.NewReader(input)), ct.Limits{MaxBytes: int64(len(input)), MaxLineBytes: 6, MaxItems: 1})
		if err != nil || len(events) != 1 {
			t.Fatalf("exact budget rejected: %v %#v", err, events)
		}
		if _, err := ct.ParseSSE(strings.NewReader(input), ct.Limits{MaxBytes: int64(len(input) - 1)}); err == nil {
			t.Fatal("byte limit was ignored")
		}
		if _, err := ct.ParseSSE(strings.NewReader(input), ct.Limits{MaxLineBytes: 5}); err == nil {
			t.Fatal("line limit was ignored")
		}
	}
	if _, err := ct.ParseSSE(strings.NewReader("data:x\n\ndata:y\n\n"), ct.Limits{MaxItems: 1}); err == nil {
		t.Fatal("item limit was ignored")
	}
}

// A bare CR must not delimit NDJSON records; LF and CRLF remain supported.
// NDJSON 的裸 CR 不能充当记录分隔符，LF 与 CRLF 保持可用。
func TestNDJSONRecordDelimiters(t *testing.T) {
	validator, err := ct.Compile([]byte(`{"type":"integer"}`), "", ct.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"1\n2\n", "1\r\n2\r\n"} {
		if err := validator.NDJSON(iotest.OneByteReader(strings.NewReader(input)), ct.Limits{}); err != nil {
			t.Fatal(err)
		}
	}
	if validator.NDJSON(strings.NewReader("1\r2\n"), ct.Limits{}) == nil {
		t.Fatal("bare CR split two NDJSON records")
	}
}
