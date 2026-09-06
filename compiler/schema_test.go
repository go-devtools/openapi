package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

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
			t.Errorf("missing %s: %s", want, b)
		}
	}
	if strings.Contains(string(b), "#/components/") || strings.Contains(string(b), `"name"`) {
		t.Fatal("standalone reference or wire field name is incorrect")
	}
	var out any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
}

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
		t.Fatal("unknown MarshalJSON was silently ignored")
	}
}

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
			t.Fatalf("uncertain enum: %s", b)
		}
	}
}

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
				t.Fatalf("display title should be %s, got %q", expression, projection.Components[key].Title)
			}
			if refs[key] {
				t.Fatal("different directions were incorrectly merged")
			}
			refs[key] = true
			raw, err := projection.Standalone()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "#/$defs/"+key) {
				t.Fatal("standalone reference was not synchronized")
			}
		}
	}
}

// Keep sorted enum values aligned with descriptions from grouped and standalone constants.
func TestEnumDescriptionsFromSource(t *testing.T) {
	project, err := Load(context.Background(), LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct{ name, values, descriptions string }{
		{"Role", `["admin","user"]`, `["Administrator","Regular user"]`},
		{"State", `[0,1,2]`, `["Pending","Running / Do not duplicate wire enum values for source aliases of the same state.","Completed"]`},
		{"Fraction", `[0.5,0.6666666666666666]`, `["One half","Two thirds"]`},
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
			t.Fatalf("enum values and descriptions are misaligned: %s %s", values, schema.Extensions["x-enum-descriptions"])
		}
	}
}
