package compiler_test

import (
	"context"
	"encoding/json"
	"errors"
	"go/types"
	"strings"
	"testing"

	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
)

// Reuse mapper-owned nested values across projections without sharing later consumer mutations.
func TestTypeMapperOwnership(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	owned := spec.Typed("object")
	owned.Properties = map[string]*spec.Schema{"name": spec.Typed("string")}
	owned.Examples = spec.Set([]any{map[string]any{"name": "original"}})
	before, err := json.Marshal(owned)
	if err != nil {
		t.Fatal(err)
	}
	mapper := func(request core.ProjectionRequest) (*spec.Schema, bool, error) {
		if request.Type == types.Typ[types.Int] {
			return owned, true, nil
		}
		return nil, false, nil
	}
	projectSchema := func(typ types.Type) *core.Projection {
		projection, err := project.Schema(core.ProjectionRequest{Type: typ, Direction: core.Output, Mappers: []core.TypeMapper{mapper}})
		if err != nil {
			t.Fatal(err)
		}
		return projection
	}
	first := projectSchema(types.NewPointer(types.Typ[types.Int]))
	second := projectSchema(types.Typ[types.Int])
	first.Root.Properties["name"].Description = "edited by the first consumer"
	first.Root.Examples.Value[0].(map[string]any)["name"] = "changed"
	after, err := json.Marshal(owned)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("mapper-owned schema changed: %s", after)
	}
	independent, err := json.Marshal(second.Root)
	if err != nil {
		t.Fatal(err)
	}
	if string(independent) != string(before) {
		t.Fatalf("another projection changed: %s", independent)
	}
	owned.Properties["name"].Description = "edited by the mapper owner"
	if second.Root.Properties["name"].Description != "" {
		t.Fatal("projection retained mapper-owned nested storage")
	}
}

// Reject missing or unserializable handled values and preserve explicit callback failures.
func TestTypeMapperInvalidResults(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/types"})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("mapper refused the type")
	for _, sample := range []struct {
		name    string
		schema  *spec.Schema
		handled bool
		err     error
		want    string
	}{
		{name: "nil handled", handled: true, want: "openapi.mapper.invalid"},
		{name: "failure", err: failure, want: failure.Error()},
		{name: "failure with schema", schema: spec.Typed("string"), handled: true, err: failure, want: failure.Error()},
	} {
		t.Run(sample.name, func(t *testing.T) {
			projection, err := project.Schema(core.ProjectionRequest{Type: types.Typ[types.Int], Direction: core.Output, Mappers: []core.TypeMapper{
				func(core.ProjectionRequest) (*spec.Schema, bool, error) {
					return sample.schema, sample.handled, sample.err
				},
			}})
			if err == nil || projection != nil || !strings.Contains(err.Error(), sample.want) {
				t.Fatalf("projection=%v err=%v", projection, err)
			}
			if sample.err != nil && !errors.Is(err, sample.err) {
				t.Fatal("mapper error identity was lost")
			}
		})
	}
	invalid := spec.Typed("object")
	invalid.Examples = spec.Set([]any{make(chan int)})
	projection, err := project.Schema(core.ProjectionRequest{Type: types.Typ[types.Int], Direction: core.Output, Mappers: []core.TypeMapper{
		func(core.ProjectionRequest) (*spec.Schema, bool, error) { return invalid, true, nil },
	}})
	if err == nil || projection != nil {
		t.Fatal("an unserializable mapper result escaped projection")
	}
}
