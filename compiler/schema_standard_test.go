package compiler_test

import (
	"context"
	"encoding/json"
	"go/types"
	"math"
	"strings"
	"testing"
	"time"

	core "github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/contracttest"
	"github.com/go-devtools/openapi/spec"
	"github.com/go-devtools/openapi/testdata/jsonstandard"
)

// Ensure projection does not invoke standard-profile fixture methods.
func standardProjection(t *testing.T, p *core.Project, name string, direction core.Direction, mappers ...core.TypeMapper) (*core.Projection, error) {
	t.Helper()
	typ, err := p.Type(name)
	if err != nil {
		t.Fatal(err)
	}
	before := jsonstandard.Calls.Load()
	projection, err := p.Schema(core.ProjectionRequest{Type: typ, Direction: direction, Mappers: mappers})
	if jsonstandard.Calls.Load() != before {
		t.Fatal("projection executed a fixture codec")
	}
	return projection, err
}

// Compile independent format and content checks for real wire samples.
func standardValidator(t *testing.T, projection *core.Projection) *contracttest.Validator {
	t.Helper()
	raw, err := projection.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(raw, "", contracttest.Options{AssertFormat: true, AssertContent: true})
	if err != nil {
		t.Fatal(err)
	}
	return validator
}

// Validate exact standard-library bytes, including wide integers and named byte slices.
func TestJSONStandardLibraryWire(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonstandard"})
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []core.Direction{core.Input, core.Output} {
		projection, err := standardProjection(t, p, "Standard", direction)
		if err != nil {
			t.Fatal(err)
		}
		object := projection.Components[strings.TrimPrefix(projection.Root.Ref, "#/components/schemas/")]
		if object.Properties["Unsigned"].Format != "uint64" || object.Properties["Signed"].Format != "int64" {
			t.Errorf("signed and unsigned formats: %s/%s", object.Properties["Signed"].Format, object.Properties["Unsigned"].Format)
		}
		validator := standardValidator(t, projection)
		for _, value := range []jsonstandard.Standard{
			{},
			{Timestamp: time.Date(2026, 9, 7, 1, 2, 3, 4, time.UTC), Duration: time.Duration(math.MaxInt64), Number: json.Number("9007199254740993"), Unsigned: math.MaxUint64, Signed: math.MinInt64, Bytes: []byte{1, 2}, Named: jsonstandard.NamedBytes{1, 2}, Alias: []byte{1, 2}, Array: [2]jsonstandard.Byte{1, 2}, Raw: json.RawMessage(`{"n":9007199254740993}`), Any: []any{false, "text", nil}},
			{Duration: -time.Nanosecond, Number: json.Number("1.5"), Bytes: []byte{}, Named: jsonstandard.NamedBytes{}, Alias: []byte{}, Raw: json.RawMessage(`[]`)},
		} {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err = validator.JSON(raw); err != nil {
				t.Errorf("%s wire=%s: %v", direction, raw, err)
			}
			var decoded jsonstandard.Standard
			if err = json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Unsigned != value.Unsigned || decoded.Signed != value.Signed || decoded.Duration != value.Duration {
				t.Fatal("standard integer values lost precision")
			}
			if value.Number != "" && decoded.Number != value.Number {
				t.Fatal("json.Number lost its exact spelling")
			}
			if value.Unsigned == math.MaxUint64 && (!strings.Contains(string(raw), `"Unsigned":18446744073709551615`) || !strings.Contains(string(raw), `"Number":9007199254740993`)) {
				t.Fatal("wide JSON numbers were rounded")
			}
			var members map[string]json.RawMessage
			if err = json.Unmarshal(raw, &members); err != nil {
				t.Fatal(err)
			}
			for field, bad := range map[string]string{"Unsigned": "-1", "Timestamp": `"invalid-date"`, "Named": `"%%%"`, "Array": "[1]", "Number": "{}"} {
				original := members[field]
				members[field] = json.RawMessage(bad)
				invalid, err := json.Marshal(members)
				if err != nil {
					t.Fatal(err)
				}
				if validator.JSON(invalid) == nil {
					t.Errorf("%s accepted invalid %s", direction, field)
				}
				members[field] = original
			}
		}
	}
}

// Base64 byte input bypasses element methods; their array-input allowance does not change the canonical schema.
func TestJSONNamedByteInput(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonstandard"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"InputBytes", "AppendSlice", "StreamSlice"} {
		projection, err := standardProjection(t, p, name, core.Input)
		if err != nil {
			t.Fatal(err)
		}
		validator := standardValidator(t, projection)
		if err = validator.JSON([]byte(`"AQI="`)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	before := jsonstandard.Calls.Load()
	var value jsonstandard.InputBytes
	if err = json.Unmarshal([]byte(`"AQI="`), &value); err != nil || len(value) != 2 || value[0] != 1 || value[1] != 2 || jsonstandard.Calls.Load() != before {
		t.Fatalf("base64=%v err=%v", value, err)
	}
	if err = json.Unmarshal([]byte(`[1,2]`), &value); err != nil || jsonstandard.Calls.Load()-before != 2 {
		t.Fatalf("array=%v err=%v", value, err)
	}
}

// Diagnose runtime-invalid format tags and preserve options actually ignored by the compatibility API.
func TestJSONStandardTagOptions(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonstandard"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name  string
		value any
	}{{"HexFormat", jsonstandard.HexFormat{}}, {"ArrayFormat", jsonstandard.ArrayFormat{}}, {"DurationFormat", jsonstandard.DurationFormat{}}, {"TimeFormat", jsonstandard.TimeFormat{}}} {
		if _, err := json.Marshal(sample.value); err == nil {
			t.Fatalf("%s unexpectedly encoded", sample.name)
		}
		for _, direction := range []core.Direction{core.Input, core.Output} {
			projection, err := standardProjection(t, p, sample.name, direction)
			if err == nil || projection != nil || !strings.Contains(err.Error(), "openapi.codec.format") {
				t.Errorf("%s/%s: projection=%v err=%v", sample.name, direction, projection, err)
			}
		}
	}
	projection, err := standardProjection(t, p, "IgnoredOptions", core.Output)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(jsonstandard.IgnoredOptions{Inline: map[string]int{"x": 1}, Unknown: map[string]int{"y": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"Inline":{"x":1},"Unknown":{"y":2}}` {
		t.Fatalf("ignored option changed nesting: %s", raw)
	}
	if err = standardValidator(t, projection).JSON(raw); err != nil {
		t.Fatal(err)
	}
}

// Describe extended interface output and input without invoking those methods in projection.
func standardMethodMapper(request core.ProjectionRequest) (*spec.Schema, bool, error) {
	named, ok := types.Unalias(request.Type).(*types.Named)
	if !ok {
		return nil, false, nil
	}
	name := named.Obj().Name()
	if request.Direction == core.Output && (name == "AppendByte" || name == "StreamByte") {
		s := spec.Typed("string")
		s.Pattern = "^byte:[0-9]+$"
		return s, true, nil
	}
	if request.Direction == core.Input && name == "StreamInput" {
		s := spec.Typed("object")
		s.Properties = map[string]*spec.Schema{"Value": spec.Typed("integer")}
		s.Required = spec.Set([]string{"Value"})
		return s, true, nil
	}
	if request.Direction == core.Output && name == "AppendMap" {
		s := spec.Typed("object", "null")
		s.AdditionalProperties = spec.Typed("string")
		s.PropertyNames = spec.Typed("string")
		s.PropertyNames.Pattern = "^byte:[0-9]+$"
		return s, true, nil
	}
	return nil, false, nil
}

// Recognize active append and streaming interfaces, including byte collections and map-key precedence.
func TestJSONExtendedMethodProfile(t *testing.T) {
	p, err := core.Load(context.Background(), core.LoadOptions{Dir: "../testdata/jsonstandard"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name      string
		direction core.Direction
		method    string
	}{
		{"AppendByte", core.Output, "AppendText"}, {"StreamByte", core.Output, "MarshalJSONTo"}, {"StreamInput", core.Input, "UnmarshalJSONFrom"}, {"AppendMap", core.Output, "AppendText"},
	} {
		projection, err := standardProjection(t, p, sample.name, sample.direction)
		if err == nil || projection != nil || !strings.Contains(err.Error(), sample.method) {
			t.Errorf("%s: projection=%v err=%v", sample.name, projection, err)
		}
	}
	for _, sample := range []struct {
		name  string
		value any
	}{
		{"AppendSlice", jsonstandard.AppendSlice{1, 2}}, {"StreamSlice", jsonstandard.StreamSlice{1, 2}},
		{"QuotedExtended", jsonstandard.QuotedExtended{Stream: 7, Append: 7}}, {"AppendMap", jsonstandard.AppendMap{7: "value"}},
	} {
		before := jsonstandard.Calls.Load()
		raw, err := json.Marshal(sample.value)
		if err != nil || jsonstandard.Calls.Load() == before {
			t.Fatalf("wire=%s err=%v", raw, err)
		}
		projection, err := standardProjection(t, p, sample.name, core.Output, standardMethodMapper)
		if err != nil {
			t.Fatal(err)
		}
		if err = standardValidator(t, projection).JSON(raw); err != nil {
			t.Errorf("%s wire=%s: %v", sample.name, raw, err)
		}
	}
	projection, err := standardProjection(t, p, "QuotedStreamInput", core.Input, standardMethodMapper)
	if err != nil {
		t.Fatal(err)
	}
	wire := []byte(`{"Value":{"Value":7}}`)
	var decoded jsonstandard.QuotedStreamInput
	if err = json.Unmarshal(wire, &decoded); err != nil || decoded.Value != 7 {
		t.Fatalf("decoded=%v err=%v", decoded, err)
	}
	if err = standardValidator(t, projection).JSON(wire); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name  string
		value any
		wire  string
	}{{"WrongStream", jsonstandard.WrongStream(7), `7`}, {"WrongAppend", jsonstandard.WrongAppend(7), `7`}, {"StreamMap", jsonstandard.StreamMap{7: "value"}, `{"7":"value"}`}} {
		before := jsonstandard.Calls.Load()
		raw, err := json.Marshal(sample.value)
		if err != nil || string(raw) != sample.wire || jsonstandard.Calls.Load() != before {
			t.Fatalf("wire=%s err=%v", raw, err)
		}
		projection, err := standardProjection(t, p, sample.name, core.Output)
		if err != nil {
			t.Fatal(err)
		}
		if err = standardValidator(t, projection).JSON(raw); err != nil {
			t.Fatal(err)
		}
	}

	before := jsonstandard.Calls.Load()
	var inputMap jsonstandard.StreamInputMap
	mapWire := []byte(`{"7":"value"}`)
	if err := json.Unmarshal(mapWire, &inputMap); err != nil || inputMap[7] != "value" || jsonstandard.Calls.Load() != before {
		t.Fatalf("JSON value methods affected a map key: value=%v err=%v", inputMap, err)
	}
	mapProjection, err := standardProjection(t, p, "StreamInputMap", core.Input)
	if err != nil {
		t.Fatal(err)
	}
	if err = standardValidator(t, mapProjection).JSON(mapWire); err != nil {
		t.Fatal(err)
	}
}
