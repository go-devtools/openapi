package compiler_test

import (
	"context"
	"encoding/json"
	"go/types"
	"strings"
	"testing"

	core "github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/contracttest"
	"github.com/go-devtools/openapi/spec"
	"github.com/go-devtools/openapi/testdata/jsonkeys"
)

// Compile standalone projections through public APIs without running key methods.
func mapKeySchema(t *testing.T, project *core.Project, name string, direction core.Direction, mappers ...core.TypeMapper) (*core.Projection, error) {
	t.Helper()
	typ, err := project.Type(name)
	if err != nil {
		t.Fatal(err)
	}
	before := jsonkeys.Calls.Load()
	schema, err := project.Schema(core.ProjectionRequest{Type: typ, Direction: direction, Mappers: mappers})
	if jsonkeys.Calls.Load() != before {
		t.Fatal("projection executed a map-key method")
	}
	return schema, err
}

// Use real standard JSON output to distinguish ignored methods from actual text codecs.
func TestJSONMapKeyMethodPrecedence(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonkeys"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name  string
		value any
		wire  string
	}{
		{"PlainMap", jsonkeys.PlainMap{7: "value"}, `{"7":"value"}`},
		{"PointerValueMap", jsonkeys.PointerValueMap{7: "value"}, `{"7":"value"}`},
		{"InputMap", jsonkeys.InputMap{7: "value"}, `{"7":"value"}`},
		{"InputStringMap", jsonkeys.InputStringMap{"wire": "value"}, `{"wire":"value"}`},
		{"WrongSignatureMap", jsonkeys.WrongSignatureMap{7: "value"}, `{"7":"value"}`},
		{"JSONValueMap", jsonkeys.JSONValueMap{7: "value"}, `{"7":"value"}`},
	} {
		t.Run(sample.name, func(t *testing.T) {
			before := jsonkeys.Calls.Load()
			wire, err := json.Marshal(sample.value)
			if err != nil || string(wire) != sample.wire {
				t.Fatalf("wire=%s err=%v", wire, err)
			}
			if jsonkeys.Calls.Load() != before {
				t.Fatal("an ignored map-key method was called")
			}
			projection, err := mapKeySchema(t, project, sample.name, core.Output)
			if err != nil {
				t.Fatal(err)
			}
			schema, err := projection.Standalone()
			if err != nil {
				t.Fatal(err)
			}
			validator, err := contracttest.Compile(schema, "", contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err = validator.JSON(wire); err != nil {
				t.Fatal(err)
			}
			if validator.JSON([]byte(`{"7":1}`)) == nil {
				t.Fatal("map value type constraint was lost")
			}
		})
	}
	// Output-only text methods do not replace ordinary integer decoding.
	projection, err := mapKeySchema(t, project, "OutputMap", core.Input)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := projection.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(schema, "", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded jsonkeys.OutputMap
	wire := []byte(`{"7":"value"}`)
	if err = json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON(wire); err != nil {
		t.Fatal(err)
	}
}

// Unknown key codecs require an explicit map projection instead of contradicting actual wire bytes.
func TestJSONCustomMapKeysRequireMapping(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonkeys"})
	if err != nil {
		t.Fatal(err)
	}
	key := jsonkeys.PointerText(7)
	for _, sample := range []struct {
		name      string
		direction core.Direction
		value     any
		input     []byte
		want      string
	}{
		{name: "OutputMap", direction: core.Output, value: jsonkeys.OutputMap{7: "value"}, want: `{"key:7":"value"}`},
		{name: "StringMap", direction: core.Output, value: jsonkeys.StringMap{"wire": "value"}, want: `{"text:wire":"value"}`},
		{name: "PointerMap", direction: core.Output, value: jsonkeys.PointerMap{&key: "value"}, want: `{"pointer:7":"value"}`},
		{name: "InputMap", direction: core.Input, value: new(jsonkeys.InputMap), input: []byte(`{"key:7":"value"}`), want: `{"key:7":"value"}`},
		{name: "InputStringMap", direction: core.Input, value: new(jsonkeys.InputStringMap), input: []byte(`{"key:7":"value"}`), want: `{"key:7":"value"}`},
	} {
		t.Run(sample.name, func(t *testing.T) {
			before := jsonkeys.Calls.Load()
			wire := sample.input
			if sample.direction == core.Output {
				wire, err = json.Marshal(sample.value)
			} else {
				err = json.Unmarshal(wire, sample.value)
			}
			if err != nil {
				t.Fatal(err)
			}
			if jsonkeys.Calls.Load() == before || string(wire) != sample.want {
				t.Fatalf("actual text codec was not applied: wire=%s calls=%d", wire, jsonkeys.Calls.Load()-before)
			}
			if sample.direction == core.Input && json.Unmarshal([]byte(`{"7":"value"}`), sample.value) == nil {
				t.Fatal("custom input codec accepted a key without its required prefix")
			}
			projection, err := mapKeySchema(t, project, sample.name, sample.direction)
			if err == nil {
				raw, exportErr := projection.Standalone()
				if exportErr != nil {
					t.Fatal(exportErr)
				}
				validator, compileErr := contracttest.Compile(raw, "", contracttest.Options{})
				if compileErr != nil {
					t.Fatal(compileErr)
				}
				t.Fatalf("custom key codec silently accepted; actual wire %s has validation result %v", wire, validator.JSON(wire))
			}
			if !strings.Contains(err.Error(), "openapi.codec.mapkey") {
				t.Fatal(err)
			}
		})
	}
}

// A centralized map mapper can describe actual custom key text without modifying the DTO or its methods.
func TestJSONCustomMapKeyMapper(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonkeys"})
	if err != nil {
		t.Fatal(err)
	}
	mapper := func(request core.ProjectionRequest) (*spec.Schema, bool, error) {
		named, ok := types.Unalias(request.Type).(*types.Named)
		if !ok || (named.Obj().Name() != "OutputMap" && named.Obj().Name() != "InputMap") {
			return nil, false, nil
		}
		schema := spec.Typed("object", "null")
		schema.AdditionalProperties = spec.Typed("string")
		schema.PropertyNames = spec.Typed("string")
		schema.PropertyNames.Pattern = `^key:[0-9]+$`
		return schema, true, nil
	}
	for _, direction := range []core.Direction{core.Input, core.Output} {
		name := "InputMap"
		if direction == core.Output {
			name = "OutputMap"
		}
		projection, err := mapKeySchema(t, project, name, direction, mapper)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := projection.Standalone()
		if err != nil {
			t.Fatal(err)
		}
		validator, err := contracttest.Compile(raw, "", contracttest.Options{})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(jsonkeys.OutputMap{7: "value"})
		if err != nil {
			t.Fatal(err)
		}
		var decoded jsonkeys.InputMap
		if err = json.Unmarshal(wire, &decoded); err != nil || decoded[7] != "value" {
			t.Fatalf("decode=%v err=%v", decoded, err)
		}
		if err = validator.JSON(wire); err != nil {
			t.Fatal(err)
		}
		if err = validator.JSON([]byte("null")); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{`{"7":"value"}`, `{"key:7":42}`} {
			if validator.JSON([]byte(bad)) == nil {
				t.Fatalf("accepted %s", bad)
			}
		}
	}
}
