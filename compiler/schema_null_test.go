package compiler_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"go/types"
	"strings"
	"testing"

	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
	"github.com/openapi-golang/openapi/testdata/jsonnull"
)

// Resolve declared source contracts through the public standalone SDK.
func nullProjection(t *testing.T, p *core.Project, name string, direction core.Direction, mappers ...core.TypeMapper) (*core.Projection, error) {
	t.Helper()
	typ, err := p.Type(name)
	if err != nil {
		t.Fatal(err)
	}
	return p.Schema(core.ProjectionRequest{Type: typ, Direction: direction, Mappers: mappers, Explain: true})
}

// Compile a standalone schema with an independent validator.
func nullValidator(t *testing.T, projection *core.Projection) *contracttest.Validator {
	t.Helper()
	raw, err := projection.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(raw, "", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return validator
}

// Restrict null at the declared use site without making unrelated references or nested fields nonnull.
func TestJSONNonnullReferenceContracts(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonnull"})
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []core.Direction{core.Input, core.Output} {
		projection, err := nullProjection(t, p, "Request", direction)
		if err != nil {
			t.Fatal(err)
		}
		validator := nullValidator(t, projection)
		zero := 0
		value := jsonnull.Request{Child: &jsonnull.Node{Value: "x"}, Anything: false, Raw: json.RawMessage(`[]`), Values: jsonnull.Names{}, Count: &zero, Must: &jsonnull.Node{Value: "required"}}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = validator.JSON(raw); err != nil {
			t.Fatal(err)
		}
		var members map[string]json.RawMessage
		if err = json.Unmarshal(raw, &members); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"Child", "Anything", "Raw", "Values", "Count", "Must"} {
			original := members[field]
			members[field] = json.RawMessage("null")
			invalid, err := json.Marshal(members)
			if err != nil {
				t.Fatal(err)
			}
			if validator.JSON(invalid) == nil {
				t.Errorf("%s accepted null for %s", direction, field)
			}
			members[field] = original
		}
		if direction == core.Input {
			if err = validator.JSON([]byte(`{"Must":{"Value":"x","Next":null}}`)); err != nil {
				t.Fatal("nonnull incorrectly implied required:", err)
			}
			if validator.JSON([]byte(`{}`)) == nil {
				t.Fatal("required declaration was lost")
			}
		}
		if len(projection.Audit) == 0 || len(projection.Origins) == 0 {
			t.Fatal("declaration audit was lost")
		}
	}
	var decoded jsonnull.Request
	if err = json.Unmarshal([]byte(`{"Must":null}`), &decoded); err != nil {
		t.Fatal("standard decoder behavior changed:", err)
	}
}

// A nonnull restriction must preserve an existing not constraint and numeric type applicability.
func TestJSONNonnullMappedUnion(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonnull"})
	if err != nil {
		t.Fatal(err)
	}
	mapper := func(request core.ProjectionRequest) (*spec.Schema, bool, error) {
		named, ok := types.Unalias(request.Type).(*types.Named)
		if !ok || named.Obj().Name() != "Custom" {
			return nil, false, nil
		}
		s := &spec.Schema{SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{spec.Typed("integer"), spec.Typed("null")}}}
		s.Not = &spec.Schema{SchemaObject: &spec.SchemaObject{Const: spec.Set[any](json.Number("7"))}}
		bound := spec.Typed("integer", "null")
		bound.Minimum = spec.Set(json.Number("0"))
		s.AllOf = []*spec.Schema{bound}
		return s, true, nil
	}
	projection, err := nullProjection(t, p, "Mapped", core.Input, mapper)
	if err != nil {
		t.Fatal(err)
	}
	validator := nullValidator(t, projection)
	if err = validator.JSON([]byte(`{"Value":8}`)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"Value":null}`, `{"Value":7}`, `{"Value":-1}`, `{"Value":"x"}`} {
		if validator.JSON([]byte(bad)) == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if _, err = nullProjection(t, p, "WrongLength", core.Input, mapper); err == nil || !strings.Contains(err.Error(), "openapi.comment.type") {
		t.Fatalf("nonnull weakened numeric type checks: %v", err)
	}
	if _, err = nullProjection(t, p, "Conflict", core.Input); err == nil || !strings.Contains(err.Error(), "openapi.comment.conflict") {
		t.Fatalf("conflicting null declarations: %v", err)
	}
	nullOnly := func(request core.ProjectionRequest) (*spec.Schema, bool, error) {
		named, ok := types.Unalias(request.Type).(*types.Named)
		if !ok || named.Obj().Name() != "Custom" {
			return nil, false, nil
		}
		return spec.Typed("null"), true, nil
	}
	if _, err = nullProjection(t, p, "Mapped", core.Input, nullOnly); err == nil || !strings.Contains(err.Error(), "openapi.comment.conflict") {
		t.Fatalf("null-only mapping accepted nonnull: %v", err)
	}

}

// Diagnose scalar byte declarations that cannot be transferred to opaque Base64 content.
func TestJSONOpaqueByteAnnotations(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonnull"})
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []core.Direction{core.Input, core.Output} {
		for _, name := range []string{"FlagBytes", "AliasBytes"} {
			projection, err := nullProjection(t, p, name, direction)
			if err == nil || projection != nil || !strings.Contains(err.Error(), "openapi.codec.byte-element") {
				t.Errorf("%s/%s: projection=%v err=%v", name, direction, projection, err)
			}
		}
		projection, err := nullProjection(t, p, "PlainBytes", direction)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(jsonnull.PlainBytes{1, 2})
		if err != nil {
			t.Fatal(err)
		}
		if err = nullValidator(t, projection).JSON(raw); err != nil {
			t.Fatal(err)
		}
		projection, err = nullProjection(t, p, "FlagArray", direction)
		if err != nil {
			t.Fatal(err)
		}
		validator := nullValidator(t, projection)
		raw, err = json.Marshal(jsonnull.FlagArray{jsonnull.First, jsonnull.Second})
		if err != nil {
			t.Fatal(err)
		}
		if err = validator.JSON(raw); err != nil {
			t.Fatal(err)
		}
		if validator.JSON([]byte(`[1,3]`)) == nil {
			t.Fatal("exposed array element enum was lost")
		}
	}
}

// Describe an enum-byte slice as encoded content through a whole-type public mapper.
func TestJSONOpaqueByteMapper(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonnull"})
	if err != nil {
		t.Fatal(err)
	}
	// Base64 encodes groups of three bytes, followed by a padded one- or two-byte tail.
	var groups, tails []string
	for _, a := range []byte{1, 2} {
		tails = append(tails, base64.StdEncoding.EncodeToString([]byte{a}))
		for _, b := range []byte{1, 2} {
			tails = append(tails, base64.StdEncoding.EncodeToString([]byte{a, b}))
			for _, c := range []byte{1, 2} {
				groups = append(groups, base64.StdEncoding.EncodeToString([]byte{a, b, c}))
			}
		}
	}
	mapper := func(request core.ProjectionRequest) (*spec.Schema, bool, error) {
		named, ok := types.Unalias(request.Type).(*types.Named)
		if !ok || named.Obj().Name() != "FlagBytes" {
			return nil, false, nil
		}
		schema := spec.Typed("string", "null")
		schema.ContentEncoding = "base64"
		schema.Pattern = "^(?:" + strings.Join(groups, "|") + ")*(?:" + strings.Join(tails, "|") + ")?$"
		return schema, true, nil
	}
	for _, direction := range []core.Direction{core.Input, core.Output} {
		projection, err := nullProjection(t, p, "FlagBytes", direction, mapper)
		if err != nil {
			t.Fatal(err)
		}
		validator := nullValidator(t, projection)
		for length := 0; length <= 6; length++ {
			for bits := 0; bits < (1 << length); bits++ {
				value := make(jsonnull.FlagBytes, length)
				for i := range value {
					value[i] = jsonnull.Flag(1 + ((bits >> i) & 1))
				}
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err = validator.JSON(raw); err != nil {
					t.Fatalf("valid enum bytes %v: %v", value, err)
				}
			}
		}
		if err = validator.JSON([]byte("null")); err != nil {
			t.Fatal(err)
		}
		for _, value := range []jsonnull.FlagBytes{{3}, {1, 3}, {1, 2, 3}, {1, 2, 1, 3}} {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if validator.JSON(raw) == nil {
				t.Fatalf("accepted undeclared byte in %v", value)
			}
		}
	}
}
