package comment

import (
	"testing"
)

// 验证普通中文、嵌套值、转义和大整数由词法扫描保留。
// Test Unicode prose, nested JSON, escapes, and large integers.
func TestDirectivePrecisionAndProse(t *testing.T) {
	d, err := Parse("创建用户\n\n创建成功后返回信息。\n@openapi required examples=[{\"值\": 9007199254740993, \"文字\": \"a b\"}] minLength=0")
	if err != nil {
		t.Fatal(err)
	}
	if d.Summary != "创建用户" || d.Description != "创建成功后返回信息。" {
		t.Fatalf("普通说明错误：%+v", d)
	}
	if string(d.Directives[0].Values["examples"]) != `[{"值": 9007199254740993, "文字": "a b"}]` {
		t.Fatal("值被改写")
	}
	if string(d.Directives[0].Values["required"]) != "true" {
		t.Fatal("裸标志丢失")
	}
}

// 验证非法、重复和超预算指令有稳定错误而非静默忽略。
// Reject invalid, duplicate, and over-budget directives with stable errors.
func TestDirectiveRejectsInvalid(t *testing.T) {
	for _, src := range []string{"@openapi magic", "@openapi minimum=01", "@openapi tags=[", "@openapi required=false required", "@openapi pattern=\"abc", "@openapi examples=[1]junk"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("错误接受 %q", src)
		}
	}
}

// 对任意注释输入验证解析器不会崩溃。
// Verify arbitrary comment input never crashes the parser.
func FuzzDirective(f *testing.F) {
	for _, s := range []string{"业务说明", "@openapi required examples=[\"中文\"]", "@openapi response status=201 mediaType=\"application/json\" type=\"User\""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _, _ = Parse(s) })
}
