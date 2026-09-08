package compiler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"
	"testing"

	core "github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/contracttest"
	"github.com/go-devtools/openapi/spec"
	"github.com/go-devtools/openapi/testdata/jsonfields"
)

// Resolve a real source type and its object schema with the public standalone exporter.
func jsonFieldProjection(t *testing.T, project *core.Project, name string, direction core.Direction) (*spec.Schema, *contracttest.Validator) {
	t.Helper()
	typ, err := project.Type(name)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := project.Schema(core.ProjectionRequest{Type: typ, Direction: direction})
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
	schema := projection.Root
	if schema.Ref != "" {
		schema = projection.Components[strings.TrimPrefix(schema.Ref, "#/components/schemas/")]
	}
	return schema, validator
}

// Validate real payload bytes before using their decoded keys in presence counterexamples.
func actualJSONObject(t *testing.T, validator *contracttest.Validator, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.JSON(raw); err != nil {
		t.Fatalf("wire=%s: %v", raw, err)
	}
	var object map[string]any
	if err = json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

// Require fields that the encoder always emits while keeping absent, null, and empty values distinct.
func TestJSONFieldOmission(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonfields"})
	if err != nil {
		t.Fatal(err)
	}
	schema, validator := jsonFieldProjection(t, project, "Fields", core.Output)
	wantRequired := []string{"Array", "Object", "PointerPresent", "Required", "a/b.$:@![]{}()?<>", "名字"}
	if !reflect.DeepEqual(schema.Required.Value, wantRequired) {
		t.Errorf("required=%v want=%v", schema.Required.Value, wantRequired)
	}
	zero := actualJSONObject(t, validator, jsonfields.Fields{})
	zeroKeys := make([]string, 0, len(zero))
	for key := range zero {
		zeroKeys = append(zeroKeys, key)
	}
	sort.Strings(zeroKeys)
	if !reflect.DeepEqual(zeroKeys, wantRequired) {
		t.Fatalf("actual zero keys=%v", zeroKeys)
	}
	for _, key := range wantRequired {
		value := zero[key]
		delete(zero, key)
		raw, err := json.Marshal(zero)
		if err != nil {
			t.Fatal(err)
		}
		if validator.JSON(raw) == nil {
			t.Errorf("accepted missing always-emitted field %s", key)
		}
		zero[key] = value
	}
	zeroValue := 0
	populated := actualJSONObject(t, validator, jsonfields.Fields{Slice: []string{"x"}, Map: map[string]string{"key": "value"}, Pointer: &zeroValue, Iface: (*int)(nil), ZeroObject: jsonfields.Inner{Name: "x"}, ZeroArray: [1]int{1}})
	if value, exists := populated["Iface"]; !exists || value != nil {
		t.Fatal("a boxed nil pointer was not emitted as a present null")
	}
	if populated["Pointer"] != float64(0) {
		t.Fatal("nonnull pointer to zero was omitted")
	}
	input, inputValidator := jsonFieldProjection(t, project, "Fields", core.Input)
	if len(input.Required.Value) != 0 || inputValidator.JSON([]byte(`{}`)) != nil {
		t.Fatal("output presence leaked into input requirements")
	}
}

// Preserve native composite JSON representations when the string option does not apply.
func TestJSONStringOptionKinds(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonfields"})
	if err != nil {
		t.Fatal(err)
	}
	value := 7
	for _, direction := range []core.Direction{core.Input, core.Output} {
		_, validator := jsonFieldProjection(t, project, "StringOptions", direction)
		actualJSONObject(t, validator, jsonfields.StringOptions{})
		actualJSONObject(t, validator, jsonfields.StringOptions{Slice: []int{}, Bytes: []byte{}, Raw: json.RawMessage(`[]`)})
		object := actualJSONObject(t, validator, jsonfields.StringOptions{Bytes: []byte{1, 2}, Raw: json.RawMessage(`{"a":1}`), Bool: true, Text: "hi", Number: 7, Pointer: &value, NamedPointer: jsonfields.NumberPointer(&value)})
		if object["Bytes"] != "AQI=" || object["Bool"] != "true" || object["Text"] != `"hi"` || object["Number"] != "7" {
			t.Fatalf("unexpected scalar wire values: %v", object)
		}
		if _, ok := object["Raw"].(map[string]any); !ok {
			t.Fatal("RawMessage was not emitted as an object")
		}
		wire, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		var decoded jsonfields.StringOptions
		if err := json.Unmarshal(wire, &decoded); err != nil || decoded.Number != 7 || decoded.Pointer == nil || *decoded.Pointer != 7 || decoded.NamedPointer == nil || *decoded.NamedPointer != 7 {
			t.Fatalf("real decoding failed: value=%+v err=%v", decoded, err)
		}
		object["Array"] = "[0]"
		raw, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		if validator.JSON(raw) == nil {
			t.Fatal("array stringification was incorrectly accepted")
		}
	}
}

// Diagnose reserved-character tag names rather than guessing implementation-specific fallback names.
func TestJSONReservedFieldNames(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonfields"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"EscapedName", "QuotedName"} {
		typ, err := project.Type(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, direction := range []core.Direction{core.Input, core.Output} {
			projection, err := project.Schema(core.ProjectionRequest{Type: typ, Direction: direction})
			if err == nil || projection != nil || !strings.Contains(err.Error(), "openapi.codec.fieldname") {
				t.Errorf("%s/%s: projection=%v err=%v", name, direction, projection, err)
			}
		}
	}
}

// Compare declared properties and presence against actual encoding across embedding choices.
func TestJSONEmbeddedFieldSelection(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonfields"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name                 string
		value                any
		properties, required []string
	}{
		{"Conflict", jsonfields.Conflict{}, []string{"LeftOnly", "RightOnly"}, []string{"LeftOnly", "RightOnly"}},
		{"Dominant", jsonfields.Dominant{TaggedLeft: jsonfields.TaggedLeft{Value: "chosen"}, TaggedRight: jsonfields.TaggedRight{Collision: 42}}, []string{"Collision"}, []string{"Collision"}},
		{"PointerEmbedded", jsonfields.PointerEmbedded{}, []string{"Collision", "LeftOnly", "Own"}, []string{"Own"}},
		{"PointerEmbedded", jsonfields.PointerEmbedded{Left: &jsonfields.Left{Collision: "x", LeftOnly: 1}}, []string{"Collision", "LeftOnly", "Own"}, []string{"Own"}},
		{"NamedEmbedded", jsonfields.NamedEmbedded{}, []string{"payload"}, []string{"payload"}},
		{"Shadow", jsonfields.Shadow{}, []string{"Collision", "LeftOnly"}, []string{"Collision", "LeftOnly"}},
		{"Repeated", jsonfields.Repeated{}, []string{}, nil},
		{"UnexportedEmbedded", jsonfields.UnexportedEmbedded{}, []string{"Exposed"}, []string{"Exposed"}},
		{"Recursive", jsonfields.Recursive{}, []string{"Count"}, []string{"Count"}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			schema, validator := jsonFieldProjection(t, project, sample.name, core.Output)
			properties := make([]string, 0, len(schema.Properties))
			for key := range schema.Properties {
				properties = append(properties, key)
			}
			sort.Strings(properties)
			if !reflect.DeepEqual(properties, sample.properties) || !reflect.DeepEqual(schema.Required.Value, sample.required) {
				t.Fatalf("properties=%v required=%v", properties, schema.Required.Value)
			}
			object := actualJSONObject(t, validator, sample.value)
			for key := range object {
				if schema.Properties[key] == nil {
					t.Fatalf("actual field missing from schema: %s", key)
				}
			}
		})
	}
}

// Describe the observed fallback name centrally without changing the source fixture.
type jsonFallbackNameCodec struct{ name string }

// Include the chosen wire name in codec identity.
func (c jsonFallbackNameCodec) Name() string { return "test-json-name/" + c.name }

// Select the fixture's one real field under the configured physical JSON name.
func (c jsonFallbackNameCodec) Fields(value *types.Struct) ([]core.WireField, error) {
	if value.NumFields() != 1 || value.Field(0).Name() != "Value" {
		return nil, fmt.Errorf("unexpected source type for fallback codec")
	}
	return []core.WireField{{Name: c.name, Field: value.Field(0)}}, nil
}

// Validate an explicit fallback codec against actual standard JSON without editing the DTO.
func TestJSONReservedFieldNameCodec(t *testing.T) {
	project, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonfields"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name, wireName string
		value          any
	}{
		{"EscapedName", "bad", jsonfields.EscapedName{Value: "present"}},
		{"QuotedName", "Value", jsonfields.QuotedName{Value: "present"}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			typ, err := project.Type(sample.name)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := project.Schema(core.ProjectionRequest{Type: typ, Direction: core.Output, Codec: jsonFallbackNameCodec{name: sample.wireName}})
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
			object := actualJSONObject(t, validator, sample.value)
			if len(object) != 1 || object[sample.wireName] != "present" {
				t.Fatalf("unexpected actual field: %v", object)
			}
			if validator.JSON([]byte(`{}`)) == nil {
				t.Fatal("explicit output field lost its required presence")
			}
		})
	}
}
