// Provide native OAS 3.2 models with lossless JSON presence semantics.
package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Use the standard default dialect rather than deriving it from the OpenAPI version.
const DefaultDialect = "https://spec.openapis.org/oas/3.1/dialect/base"

// Distinguish absence from explicit zero, empty collections, and null.
type Optional[T any] struct {
	Value   T
	Present bool
}

// Construct an explicitly present value.
func Set[T any](value T) Optional[T] { return Optional[T]{Value: value, Present: true} }

// Support encoding/json omitzero presence checks.
func (v Optional[T]) IsZero() bool { return !v.Present }

// Serialize a present value without dropping JSON zero values.
func (v Optional[T]) MarshalJSON() ([]byte, error) { return json.Marshal(v.Value) }

// Restore explicit presence from JSON.
func (v *Optional[T]) UnmarshalJSON(b []byte) error {
	v.Present = true
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(&v.Value)
}

// Represent a single Schema type or a type union.
type Types []string

// Encode one type as a string and multiple types as an array.
func (t Types) MarshalJSON() ([]byte, error) {
	if len(t) == 1 {
		return json.Marshal(t[0])
	}
	return json.Marshal([]string(t))
}

// Accept standard single-type and type-array forms.
func (t *Types) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = Types{s}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(t))
}

// Store x- extensions as valid raw JSON.
type Extensions map[string]json.RawMessage

// Represent a Reference Object separately from Schema references.
type Reference struct {
	Ref         string `json:"$ref"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
}

// Distinguish a reference from an inline standard object.
type RefOr[T any] struct {
	Reference *Reference
	Value     *T
}

// Construct an inline standard object.
func Inline[T any](v T) RefOr[T] { return RefOr[T]{Value: &v} }

// Construct a standard Reference Object.
func Ref[T any](ref string) RefOr[T] { return RefOr[T]{Reference: &Reference{Ref: ref}} }

// Reject empty or conflicting branches during serialization.
func (r RefOr[T]) MarshalJSON() ([]byte, error) {
	if (r.Reference == nil) == (r.Value == nil) {
		return nil, fmt.Errorf("reference must select exactly one branch")
	}
	if r.Reference != nil {
		return json.Marshal(r.Reference)
	}
	return json.Marshal(r.Value)
}

// Recognize reference objects by $ref without treating siblings as inline objects.
func (r *RefOr[T]) UnmarshalJSON(b []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	if keys == nil {
		return fmt.Errorf("standard object must not be null")
	}
	*r = RefOr[T]{}
	if _, ok := keys["$ref"]; ok {
		r.Reference = &Reference{}
		return json.Unmarshal(b, r.Reference)
	}
	r.Value = new(T)
	return json.Unmarshal(b, r.Value)
}

// Merge extensions while refusing to replace standard fields.
func marshalExtensions(v any, ext Extensions) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(ext) == 0 {
		return b, nil
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	for k, v := range ext {
		if len(k) < 2 || k[:2] != "x-" {
			return nil, fmt.Errorf("extension must start with x-: %s", k)
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("extension value is not JSON: %s", k)
		}
		obj[k] = v
	}
	return json.Marshal(obj)
}

// Decode extension fields separately from the standard typed fields.
func readExtensions(b []byte) (Extensions, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var ext Extensions
	for k, v := range raw {
		if len(k) >= 2 && k[:2] == "x-" {
			if ext == nil {
				ext = Extensions{}
			}
			ext[k] = bytes.Clone(v)
		}
	}
	return ext, nil
}
