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
	"github.com/go-devtools/openapi/testdata/jsonmethods"
)

// Capture runtime counters without invoking any business codec.
func jsonMethodCounts() [3]int64 {
	return [3]int64{jsonmethods.JSONCalls.Load(), jsonmethods.TextCalls.Load(), jsonmethods.IgnoredCalls.Load()}
}

// Check method-free public projection before exporting its standalone contract.
func jsonMethodSchema(t *testing.T, project *core.Project, name string, direction core.Direction, mappers ...core.TypeMapper) (*core.Projection, error) {
	t.Helper()
	typ, err := project.Type(name)
	if err != nil {
		t.Fatal(err)
	}
	before := jsonMethodCounts()
	projection, err := project.Schema(core.ProjectionRequest{Type: typ, Direction: direction, Mappers: mappers})
	if jsonMethodCounts() != before {
		t.Fatal("projection ran a business codec")
	}
	return projection, err
}

// Compile an independent validator for a successfully exported projection.
func jsonMethodValidator(t *testing.T, projection *core.Projection) *contracttest.Validator {
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

// Ignore non-interface signatures and methods belonging only to the opposite wire direction.
func TestJSONValueMethodSignatures(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonmethods"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name      string
		direction core.Direction
		value     any
		wire      string
	}{
		{"WrongJSON", core.Output, jsonmethods.WrongJSON(7), `7`},
		{"WrongText", core.Output, jsonmethods.WrongText("plain"), `"plain"`},
		{"Variadic", core.Output, jsonmethods.Variadic(7), `7`},
		{"InputJSON", core.Output, jsonmethods.InputJSON(7), `7`},
		{"InputText", core.Output, jsonmethods.InputText("plain"), `"plain"`},
		{"WrongInputJSON", core.Input, new(jsonmethods.WrongInputJSON), `7`},
		{"WrongInputText", core.Input, new(jsonmethods.WrongInputText), `"plain"`},
		{"WrongNamedInput", core.Input, new(jsonmethods.WrongNamedInput), `7`},
		{"OutputJSON", core.Input, new(jsonmethods.OutputJSON), `7`},
		{"OutputText", core.Input, new(jsonmethods.OutputText), `"plain"`},
	} {
		t.Run(sample.name, func(t *testing.T) {
			before := jsonMethodCounts()
			wire := []byte(sample.wire)
			if sample.direction == core.Output {
				wire, err = json.Marshal(sample.value)
			} else {
				err = json.Unmarshal(wire, sample.value)
			}
			if err != nil || string(wire) != sample.wire || jsonMethodCounts() != before {
				t.Fatalf("wire=%s err=%v counts=%v", wire, err, jsonMethodCounts())
			}
			projection, err := jsonMethodSchema(t, project, sample.name, sample.direction)
			if err != nil {
				t.Fatal(err)
			}
			validator := jsonMethodValidator(t, projection)
			if err = validator.JSON(wire); err != nil {
				t.Fatal(err)
			}
			if validator.JSON([]byte(`{}`)) == nil {
				t.Fatal("scalar contract lost its wire type")
			}
		})
	}
}

// Require mappings for effective custom interfaces and report JSON before its competing text interface.
func TestJSONValueMethodsRequireMapping(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonmethods"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name      string
		direction core.Direction
		method    string
	}{
		{"OutputJSON", core.Output, "MarshalJSON"}, {"OutputText", core.Output, "MarshalText"},
		{"InputJSON", core.Input, "UnmarshalJSON"}, {"InputText", core.Input, "UnmarshalText"},
		{"Both", core.Output, "MarshalJSON"}, {"Both", core.Input, "UnmarshalJSON"},
	} {
		projection, err := jsonMethodSchema(t, project, sample.name, sample.direction)
		if err == nil || projection != nil || !strings.Contains(err.Error(), "openapi.codec.custom") || !strings.Contains(err.Error(), sample.method) {
			t.Errorf("%s/%s: projection=%v err=%v", sample.name, sample.direction, projection, err)
		}
	}
}

// Describe the actual object returned by the fixture's JSON methods.
func jsonMethodObject() *spec.Schema {
	schema := spec.Typed("object")
	schema.Properties = map[string]*spec.Schema{"Value": spec.Typed("integer")}
	schema.Required = spec.Set([]string{"Value"})
	return schema
}

// Keep custom wire shapes centralized and explicitly direction-dependent.
func jsonMethodMapper(request core.ProjectionRequest) (*spec.Schema, bool, error) {
	named, ok := types.Unalias(request.Type).(*types.Named)
	if !ok {
		return nil, false, nil
	}
	name := named.Obj().Name()
	if name == "Both" || request.Direction == core.Output && (name == "OutputJSON" || name == "PointerJSON") || request.Direction == core.Input && name == "InputJSON" {
		return jsonMethodObject(), true, nil
	}
	if request.Direction == core.Output && name == "OutputText" || request.Direction == core.Input && name == "InputText" {
		schema := spec.Typed("string")
		schema.Pattern = "^label:"
		return schema, true, nil
	}
	return nil, false, nil
}

// Custom methods own the quoted field's representation; JSON takes precedence over Text.
func TestJSONMappedMethodPrecedence(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonmethods"})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(jsonmethods.OutputFields{JSON: 7, Text: "hello"})
	if err != nil || string(wire) != `{"JSON":{"Value":7},"Text":"label:hello"}` {
		t.Fatalf("wire=%s err=%v", wire, err)
	}
	var decoded jsonmethods.InputFields
	if err = json.Unmarshal(wire, &decoded); err != nil || decoded.JSON != 7 || decoded.Text != "hello" {
		t.Fatalf("decode=%v err=%v", decoded, err)
	}
	for _, sample := range []struct {
		name      string
		direction core.Direction
	}{{"OutputFields", core.Output}, {"InputFields", core.Input}} {
		projection, err := jsonMethodSchema(t, project, sample.name, sample.direction, jsonMethodMapper)
		if err != nil {
			t.Fatal(err)
		}
		validator := jsonMethodValidator(t, projection)
		if err = validator.JSON(wire); err != nil {
			t.Errorf("%s: real custom wire rejected: %v", sample.name, err)
		}
		for _, invalid := range []string{`{"JSON":"7","Text":"label:hello"}`, `{"JSON":{"Value":7},"Text":"hello"}`} {
			if validator.JSON([]byte(invalid)) == nil {
				t.Errorf("%s accepted %s", sample.name, invalid)
			}
			if sample.direction == core.Input && json.Unmarshal([]byte(invalid), new(jsonmethods.InputFields)) == nil {
				t.Errorf("actual input codec accepted %s", invalid)
			}
		}
	}
	before := jsonMethodCounts()
	bothWire, err := json.Marshal(jsonmethods.Both(7))
	if err != nil {
		t.Fatal(err)
	}
	var both jsonmethods.Both
	if err = json.Unmarshal(bothWire, &both); err != nil || both != 7 {
		t.Fatalf("value=%v err=%v", both, err)
	}
	after := jsonMethodCounts()
	if after[0]-before[0] != 2 || after[1] != before[1] {
		t.Fatal("JSON did not take precedence over Text")
	}
	for _, direction := range []core.Direction{core.Input, core.Output} {
		projection, err := jsonMethodSchema(t, project, "Both", direction, jsonMethodMapper)
		if err != nil {
			t.Fatal(err)
		}
		if err = jsonMethodValidator(t, projection).JSON(bothWire); err != nil {
			t.Fatal(err)
		}
	}
}

// Require a containing-type mapping when pointer-only methods can change a quoted field's shape.
func TestJSONPointerMethodAddressability(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonmethods"})
	if err != nil {
		t.Fatal(err)
	}
	value := jsonmethods.PointerFields{Value: 7}
	valueWire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	pointerWire, err := json.Marshal(&value)
	if err != nil {
		t.Fatal(err)
	}
	if string(valueWire) != `{"Value":"7"}` || string(pointerWire) != `{"Value":{"Value":7}}` {
		t.Fatalf("value=%s pointer=%s", valueWire, pointerWire)
	}
	projection, err := jsonMethodSchema(t, project, "PointerFields", core.Output, jsonMethodMapper)
	if err == nil || projection != nil || !strings.Contains(err.Error(), "openapi.codec.addressability") {
		t.Errorf("ambiguous quoted field: projection=%v err=%v", projection, err)
	}
	mapper := func(request core.ProjectionRequest) (*spec.Schema, bool, error) {
		named, ok := types.Unalias(request.Type).(*types.Named)
		if !ok || named.Obj().Name() != "PointerFields" {
			return nil, false, nil
		}
		text := spec.Typed("string")
		text.Pattern = "^[0-9]+$"
		schema := spec.Typed("object")
		schema.Properties = map[string]*spec.Schema{"Value": {SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{text, jsonMethodObject()}}}}
		schema.Required = spec.Set([]string{"Value"})
		return schema, true, nil
	}
	projection, err = jsonMethodSchema(t, project, "PointerFields", core.Output, mapper)
	if err != nil {
		t.Fatal(err)
	}
	validator := jsonMethodValidator(t, projection)
	for _, raw := range [][]byte{valueWire, pointerWire} {
		if err = validator.JSON(raw); err != nil {
			t.Fatal(err)
		}
	}
	if validator.JSON([]byte(`{"Value":7}`)) == nil {
		t.Fatal("mapped addressability alternatives accepted an impossible unquoted scalar")
	}
}
