package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// 验证真实零 tag 类型、注释、递归、bytes 和 map key 投影。
func TestProjectSchemaFromSource(t *testing.T) {
	p, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := p.Type("Request")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := p.Schema(ProjectionRequest{Type: typ, Direction: Input, MediaType: "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := projected.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Name"`, `"minLength":3`, `"required":["Name"]`, `"contentEncoding":"base64"`, `"date-time"`, `"$defs"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("缺少 %s：%s", want, b)
		}
	}
	if strings.Contains(string(b), "#/components/") || strings.Contains(string(b), `"name"`) {
		t.Fatal("独立引用或网络字段名错误")
	}
	var out any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
}

// 验证未知用户序列化返回诊断而不是执行用户函数。
func TestCustomCodecRequiresMapper(t *testing.T) {
	p, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := p.Type("Custom")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Schema(ProjectionRequest{Type: typ, Direction: Output, MediaType: "application/json"}); err == nil {
		t.Fatal("未知 MarshalJSON 被静默忽略")
	}
}

// 验证泛型具体化与显式枚举能通过同一公共入口使用。
func TestGenericAndExplicitEnum(t *testing.T) {
	p, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Page[Request]", "Role"} {
		typ, err := p.Type(name)
		if err != nil {
			t.Fatal(err)
		}
		s, err := p.Schema(ProjectionRequest{Type: typ, Direction: Output, MediaType: "application/json"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := s.Standalone()
		if err != nil {
			t.Fatal(err)
		}
		if name == "Role" && !strings.Contains(string(b), `"enum":["admin","user"]`) {
			t.Fatalf("枚举不确定：%s", b)
		}
	}
}
