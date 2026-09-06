package compiler

import (
	"context"
	"strings"
	"testing"
)

// Reproduce conflicting annotations from real source before document generation or runtime.
// 从真实源码复现注释矛盾，防止把错误推迟到文档生成或运行阶段。
func TestSourceAnnotationConflicts(t *testing.T) {
	p, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, code string }{
		{"StringMinimum", "openapi.comment.type"}, {"NamedMinimum", "openapi.comment.type"},
		{"ReversedLength", "openapi.comment.range"}, {"ReversedNumber", "openapi.comment.range"},
		{"ChangedArray", "openapi.comment.derived"}, {"ChangedUnsigned", "openapi.comment.derived"},
		{"BadRequired", "openapi.comment.value"}, {"ChangedType", "openapi.comment.context"},
		{"NullLength", "openapi.comment.value"}, {"NullReadOnly", "openapi.comment.value"}, {"ZeroMultiple", "openapi.comment.value"},
		{"PreciseReversedNumber", "openapi.comment.range"}, {"ReferencedRange", "openapi.comment.range"}, {"ExponentBudget", "openapi.comment.budget"},
		{"OutputWriteOnly", "openapi.schema.writeOnly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typ, err := p.Type(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Schema(ProjectionRequest{Type: typ, Direction: Output})
			if err == nil || !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
}

// Preserve valid bounds and fixed-array facts without duplicating nullable union members.
// 保留合法范围与固定数组事实，nullable 不能产生重复的联合类型。
func TestSourceAnnotationValidConstraints(t *testing.T) {
	p, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := p.Type("ValidConstraints")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := p.Schema(ProjectionRequest{Type: typ, Direction: Output})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := projection.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"null","null"`) {
		t.Fatalf("nullable is duplicated: %s", raw)
	}
}

// Do not report type conflicts for open fields merely because they lack a fixed type keyword.
// 开放字段不应因为没有固定 type 关键字而误报类型冲突。
func TestOpenAnnotationConstraint(t *testing.T) {
	p, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := p.Type("OpenConstraints")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Schema(ProjectionRequest{Type: typ, Direction: Input}); err != nil {
		t.Fatal(err)
	}
}
