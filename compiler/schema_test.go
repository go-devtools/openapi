package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// 验证真实零 tag 类型、注释、递归、bytes 和 map key 投影。
// Verify tag-free types, annotations, recursion, bytes, and map-key projections.
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
// Require diagnostics instead of executing unknown user serialization methods.
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
// Verify generic instances and explicit enums through the public API.
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

// 展示名称与内部组件身份分离；不同投影可区分但页面标题不带标识。
// Separate clean display titles from distinct internal projection identities.
func TestReadableSchemaTitles(t *testing.T) {
	p, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expression := range []string{"Request", "Page[Request]"} {
		typ, err := p.Type(expression)
		if err != nil {
			t.Fatal(err)
		}
		refs := map[string]bool{}
		for _, direction := range []Direction{Input, Output} {
			projection, err := p.Schema(ProjectionRequest{Type: typ, Direction: direction})
			if err != nil {
				t.Fatal(err)
			}
			key := strings.TrimPrefix(projection.Root.Ref, "#/components/schemas/")
			if projection.Components[key].Title != expression {
				t.Fatalf("展示标题应为 %s，实际为 %q", expression, projection.Components[key].Title)
			}
			if refs[key] {
				t.Fatal("不同方向被错误合并")
			}
			refs[key] = true
			raw, err := projection.Standalone()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "#/$defs/"+key) {
				t.Fatal("独立引用未同步")
			}
		}
	}
}

// 枚举值排序与说明同步，读取分组常量和单独常量的源码注释。
// Keep sorted enum values aligned with descriptions from grouped and standalone constants.
func TestEnumDescriptionsFromSource(t *testing.T) {
	project, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct{ name, values, descriptions string }{
		{"Role", `["admin","user"]`, `["管理员","普通用户"]`},
		{"State", `[0,1,2]`, `["待处理","执行中 / 相同状态的源码别名不重复生成网络枚举。","已完成"]`},
		{"Fraction", `[0.5,0.6666666666666666]`, `["一半","三分之二"]`},
	} {
		typ, err := project.Type(sample.name)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := project.Schema(ProjectionRequest{Type: typ, Direction: Output})
		if err != nil {
			t.Fatal(err)
		}
		schema := projection.Components[strings.TrimPrefix(projection.Root.Ref, "#/components/schemas/")]
		values, err := json.Marshal(schema.Enum.Value)
		if err != nil {
			t.Fatal(err)
		}
		if string(values) != sample.values || string(schema.Extensions["x-enum-descriptions"]) != sample.descriptions {
			t.Fatalf("枚举值与说明不对应：%s %s", values, schema.Extensions["x-enum-descriptions"])
		}
	}
}
