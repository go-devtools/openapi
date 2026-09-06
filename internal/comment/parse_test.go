package comment

import (
	"testing"
)

// Test Unicode prose, nested JSON, escapes, and large integers.
// 验证普通中文、嵌套值、转义和大整数由词法扫描保留。
func TestDirectivePrecisionAndProse(t *testing.T) {
	d, err := Parse("\u521b\u5efa\u7528\u6237\n\n\u521b\u5efa\u6210\u529f\u540e\u8fd4\u56de\u4fe1\u606f\u3002\n@openapi required examples=[{\"\u503c\": 9007199254740993, \"\u6587\u5b57\": \"a b\"}] minLength=0")
	if err != nil {
		t.Fatal(err)
	}
	if d.Summary != "\u521b\u5efa\u7528\u6237" || d.Description != "\u521b\u5efa\u6210\u529f\u540e\u8fd4\u56de\u4fe1\u606f\u3002" {
		t.Fatalf("ordinary description is incorrect: %+v", d)
	}
	if string(d.Directives[0].Values["examples"]) != "[{\"\u503c\": 9007199254740993, \"\u6587\u5b57\": \"a b\"}]" {
		t.Fatal("value was changed")
	}
	if string(d.Directives[0].Values["required"]) != "true" {
		t.Fatal("bare flag was lost")
	}
}

// Reject invalid, duplicate, and over-budget directives with stable errors.
// 验证非法、重复和超预算指令有稳定错误而非静默忽略。
func TestDirectiveRejectsInvalid(t *testing.T) {
	for _, src := range []string{"@openapi magic", "@openapi minimum=01", "@openapi tags=[", "@openapi required=false required", "@openapi pattern=\"abc", "@openapi examples=[1]junk"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("incorrectly accepted %q", src)
		}
	}
}

// Verify arbitrary comment input never crashes the parser.
// 对任意注释输入验证解析器不会崩溃。
func FuzzDirective(f *testing.F) {
	for _, s := range []string{"\u4e1a\u52a1\u8bf4\u660e", "@openapi required examples=[\"\u4e2d\u6587\"]", "@openapi response status=201 mediaType=\"application/json\" type=\"User\""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _, _ = Parse(s) })
}
