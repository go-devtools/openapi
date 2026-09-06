package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Preserve absence and both boolean values across every optional native boolean field.
func TestNativeBooleanPresence(t *testing.T) {
	cases := []struct {
		model  any
		fields []string
	}{
		{Operation{}, []string{"deprecated"}},
		{Parameter{}, []string{"required", "deprecated", "allowEmptyValue", "allowReserved", "explode"}},
		{RequestBody{}, []string{"required"}},
		{Encoding{}, []string{"allowReserved", "explode"}},
		{Header{}, []string{"required", "deprecated", "explode"}},
		{SchemaObject{}, []string{"deprecated", "readOnly", "writeOnly", "uniqueItems"}},
		{XML{}, []string{"attribute", "wrapped"}},
		{SecurityScheme{}, []string{"deprecated"}},
	}
	for _, tc := range cases {
		for _, field := range tc.fields {
			for _, value := range []string{"absent", "false", "true"} {
				t.Run(reflect.TypeOf(tc.model).Name()+"/"+field+"/"+value, func(t *testing.T) {
					input := `{}`
					if value != "absent" {
						input = `{"` + field + `":` + value + `}`
					}
					target := reflect.New(reflect.TypeOf(tc.model)).Interface()
					if err := json.Unmarshal([]byte(input), target); err != nil {
						t.Fatal(err)
					}
					raw, err := json.Marshal(target)
					if err != nil {
						t.Fatal(err)
					}
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(raw, &fields); err != nil {
						t.Fatal(err)
					}
					result, present := fields[field]
					if present != (value != "absent") || present && string(result) != value {
						t.Fatalf("boolean presence changed: %s -> %s", input, raw)
					}
				})
			}
		}
	}
}

// Reject null for nonnullable scalar optionals without corrupting the previous value.
func TestOptionalScalarNullAndAtomicDecode(t *testing.T) {
	for _, current := range []any{Set(true), Set("kept"), Set(uint64(9)), Set(json.Number("1.25"))} {
		for _, input := range []string{`null`, `{"wrong":true}`} {
			target := reflect.New(reflect.TypeOf(current))
			target.Elem().Set(reflect.ValueOf(current))
			if err := json.Unmarshal([]byte(input), target.Interface()); err == nil {
				t.Errorf("accepted invalid scalar %s into %T", input, current)
			}
			if !reflect.DeepEqual(target.Elem().Interface(), current) {
				t.Errorf("failed decode changed %T: %+v", current, target.Elem().Interface())
			}
		}
	}
	var logical Optional[any]
	if err := json.Unmarshal([]byte(`null`), &logical); err != nil || !logical.Present || logical.Value != nil {
		t.Fatalf("logical null lost: %+v %v", logical, err)
	}
	var huge Optional[any]
	if err := json.Unmarshal([]byte(`9007199254740993`), &huge); err != nil || huge.Value != json.Number("9007199254740993") {
		t.Fatalf("integer precision lost: %+v %v", huge, err)
	}
	var nullable Optional[*bool]
	if err := json.Unmarshal([]byte(`null`), &nullable); err != nil || !nullable.Present || nullable.Value != nil {
		t.Fatalf("pointer null lost: %+v %v", nullable, err)
	}
}

// Preserve the empty JSON property name and explicit legacy XML values in typed construction.
func TestTypedNativeObjectPresence(t *testing.T) {
	discriminator, err := json.Marshal(Discriminator{PropertyName: ""})
	if err != nil || string(discriminator) != `{"propertyName":""}` {
		t.Fatalf("empty property name lost: %s %v", discriminator, err)
	}
	legacy, err := json.Marshal(XML{Attribute: Set(false), Wrapped: Set(false)})
	if err != nil || string(legacy) != `{"attribute":false,"wrapped":false}` {
		t.Fatalf("explicit XML false lost: %s %v", legacy, err)
	}
	example, err := json.Marshal(Example{DataValue: Set[any](nil), SerializedValue: Set("")})
	if err != nil || string(example) != `{"dataValue":null,"serializedValue":""}` {
		t.Fatalf("example presence lost: %s %v", example, err)
	}
}
