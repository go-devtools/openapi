// Provide actual JSON map-key methods for directional wire contract tests.
package jsonkeys

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// Count real codec calls so projection can prove it never executes business methods.
var Calls atomic.Int64

// Keep ordinary named integer keys available as the baseline.
type Plain int

// Override integer key encoding with textual keys.
type OutputText int

// Encode an actual map key independently of schema generation.
func (v OutputText) MarshalText() ([]byte, error) {
	Calls.Add(1)
	return []byte("key:" + strconv.Itoa(int(v))), nil
}

// Keep a pointer-only marshaler that is unavailable on non-addressable integer map keys.
type PointerText int

// Encode only actual pointer keys, never non-addressable PointerText values.
func (v *PointerText) MarshalText() ([]byte, error) {
	Calls.Add(1)
	return []byte("pointer:" + strconv.Itoa(int(*v))), nil
}

// The default Go 1.27 JSON engine calls text methods on named string keys.
type StringText string

// Encode string keys through the method selected by the actual standard JSON implementation.
func (v StringText) MarshalText() ([]byte, error) {
	Calls.Add(1)
	return []byte("text:" + string(v)), nil
}

// Decode named integer keys using a custom text grammar.
type InputText int

// Decode actual text keys without a numeric object-key assumption.
func (v *InputText) UnmarshalText(raw []byte) error {
	Calls.Add(1)
	if !strings.HasPrefix(string(raw), "key:") {
		return fmt.Errorf("missing key prefix")
	}
	number, err := strconv.Atoi(strings.TrimPrefix(string(raw), "key:"))
	*v = InputText(number)
	return err
}

// Custom string input methods precede the underlying key kind.
type InputString string

// Require a key prefix when decoding an actual string key.
func (v *InputString) UnmarshalText(raw []byte) error {
	Calls.Add(1)
	if !strings.HasPrefix(string(raw), "key:") {
		return fmt.Errorf("missing key prefix")
	}
	*v = InputString(raw)
	return nil
}

// Same-name methods with different signatures do not implement encoding.TextMarshaler.
type WrongSignature int

// Deliberately expose a different method signature to test exact interface matching.
func (WrongSignature) MarshalText() string { Calls.Add(1); return "unused" }

// JSON value methods do not participate in object-key encoding.
type JSONValue int

// This method must not run for a map key.
func (JSONValue) MarshalJSON() ([]byte, error) { Calls.Add(1); return []byte(`"unused"`), nil }

// Name each actual map so the public compiler resolves its source identity.
type PlainMap map[Plain]string
type OutputMap map[OutputText]string
type PointerValueMap map[PointerText]string
type PointerMap map[*PointerText]string
type StringMap map[StringText]string
type InputMap map[InputText]string
type InputStringMap map[InputString]string
type WrongSignatureMap map[WrongSignature]string
type JSONValueMap map[JSONValue]string
