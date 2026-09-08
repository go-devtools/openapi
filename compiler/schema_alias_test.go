package compiler_test

import (
	"context"
	"encoding/json"
	"errors"
	"go/types"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
	"strings"
	"testing"

	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/testdata/jsonalias"
)

// Project real aliases without normalizing away their declaration metadata.
func aliasProjection(t *testing.T, p *core.Project, name string) (*core.Projection, error) {
	t.Helper()
	typ, err := p.Type(name)
	if err != nil {
		t.Fatal(err)
	}
	return p.Schema(core.ProjectionRequest{Type: typ, Direction: core.Input, Explain: true})
}

// Export an alias contract and validate it independently.
func aliasValidator(t *testing.T, projection *core.Projection) *contracttest.Validator {
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

// Preserve scalar, chained, explicit-enum, and instantiated generic alias declarations.
func TestJSONAliasDeclarations(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonalias"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name  string
		value any
		bad   []string
	}{
		{"Label", jsonalias.Label("hello"), []string{`"x"`}},
		{"Short", jsonalias.Short("hello"), []string{`"x"`, `"longer"`}},
		{"Relaxed", jsonalias.Relaxed("hello"), []string{`"x"`}},
		{"Color", jsonalias.Color("red"), []string{`"green"`}},
		{"Batch[string]", jsonalias.Batch[string]{"x"}, []string{`[]`, `[1]`}},
		{"Batch[int]", jsonalias.Batch[int]{1}, []string{`[]`, `["x"]`}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			projection, err := aliasProjection(t, p, sample.name)
			if err != nil {
				t.Fatal(err)
			}
			if projection.Root.SchemaObject == nil || projection.Root.Title != sample.name {
				t.Errorf("alias display title was lost: %v", projection.Root)
			}
			validator := aliasValidator(t, projection)
			raw, err := json.Marshal(sample.value)
			if err != nil {
				t.Fatal(err)
			}
			if err = validator.JSON(raw); err != nil {
				t.Fatal(err)
			}
			for _, bad := range sample.bad {
				if validator.JSON([]byte(bad)) == nil {
					t.Errorf("%s accepted %s", sample.name, bad)
				}
			}
			found := false
			for _, origin := range projection.Origins {
				if strings.Contains(origin.Source.Symbol, "."+strings.Split(sample.name, "[")[0]) && len(origin.Declarations) > 0 {
					found = true
				}
			}
			if !found {
				t.Error("alias declaration origin was lost")
			}
		})
	}
}

// Alias constraints are conjunctive and do not mutate the target component or unrelated uses.
func TestJSONAliasIsolationAndRecursion(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonalias"})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := aliasProjection(t, p, "Pair")
	if err != nil {
		t.Fatal(err)
	}
	validator := aliasValidator(t, projection)
	if err = validator.JSON([]byte(`{"Restricted":{"Value":"x"},"Plain":{},"Label":"good","Text":""}`)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"Restricted":{}}`, `{"Label":"x"}`} {
		if validator.JSON([]byte(bad)) == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	base, err := aliasProjection(t, p, "Base")
	if err != nil {
		t.Fatal(err)
	}
	if err = aliasValidator(t, base).JSON([]byte(`{}`)); err != nil {
		t.Fatal("alias mutated its target:", err)
	}
	recursive, err := aliasProjection(t, p, "NodeAlias")
	if err != nil {
		t.Fatal(err)
	}
	validator = aliasValidator(t, recursive)
	value := jsonalias.NodeAlias{Value: "x", Next: &jsonalias.NodeAlias{Value: "y"}}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{}`, `{"Next":{}}`} {
		if validator.JSON([]byte(bad)) == nil {
			t.Errorf("recursive alias accepted %s", bad)
		}
	}
}

// Contradictory alias bounds and ambiguous auto-enum declarations must be actionable diagnostics.
func TestJSONAliasDiagnostics(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonalias"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct{ name, code string }{{"Impossible", "openapi.comment.range"}, {"AmbiguousEnum", "openapi.schema.alias-enum"}} {
		projection, err := aliasProjection(t, p, sample.name)
		if err == nil || projection != nil || !strings.Contains(err.Error(), sample.code) {
			t.Errorf("%s: err=%v", sample.name, err)
		}
		var report openapi.Report
		if !errors.As(err, &report) || len(report.Diagnostics) != 1 {
			t.Fatalf("%s: missing structured declaration diagnostic: %v", sample.name, err)
		}
		diagnostic := report.Diagnostics[0]
		if diagnostic.Code != sample.code || diagnostic.Source.File == "" || diagnostic.Source.Line == 0 || !strings.HasSuffix(diagnostic.Source.Symbol, "."+sample.name) {
			t.Errorf("%s: declaration source was lost: %+v", sample.name, diagnostic)
		}
	}
}

// Preserve output declarations while retaining explicit mapper precedence and the shared type budget.
func TestJSONAliasProjectionBoundaries(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonalias"})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := p.Type("Short")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := p.Schema(core.ProjectionRequest{Type: typ, Direction: core.Output})
	if err != nil {
		t.Fatal(err)
	}
	validator := aliasValidator(t, projection)
	raw, err := json.Marshal(jsonalias.Short("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON(raw); err != nil {
		t.Fatal(err)
	}
	if validator.JSON([]byte(`"x"`)) == nil {
		t.Fatal("output alias lost its inherited minimum length")
	}
	if projection, err = p.Schema(core.ProjectionRequest{Type: typ, Direction: core.Input, MaxTypes: 1}); err == nil || projection != nil || !strings.Contains(err.Error(), "openapi.schema.budget") {
		t.Fatalf("alias expansion bypassed its budget: %v", err)
	}
	projection, err = p.Schema(core.ProjectionRequest{Type: typ, Direction: core.Output, Mappers: []core.TypeMapper{
		func(request core.ProjectionRequest) (*spec.Schema, bool, error) {
			alias, ok := request.Type.(*types.Alias)
			if !ok || alias.Obj().Name() != "Short" {
				return nil, false, nil
			}
			return spec.Typed("integer"), true, nil
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	validator = aliasValidator(t, projection)
	if err = validator.JSON([]byte(`7`)); err != nil {
		t.Fatal(err)
	}
	if validator.JSON(raw) == nil {
		t.Fatal("alias annotation replaced an explicit mapper")
	}
}
