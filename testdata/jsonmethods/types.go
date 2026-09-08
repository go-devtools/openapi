// Expose actual codec methods for signature, direction, and field-precedence contracts.
package jsonmethods

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
)

// Count real runtime calls independently of compiler traversal.
var JSONCalls, TextCalls, IgnoredCalls atomic.Int64

// Keep invalid method signatures distinct from the standard codec interfaces.
type WrongJSON int
type WrongText string
type WrongInputJSON int
type WrongInputText string
type Variadic int
type NamedBytes []byte
type WrongNamedInput int

// A string result does not implement json.Marshaler.
func (WrongJSON) MarshalJSON() string { IgnoredCalls.Add(1); return "unused" }

// Extra parameters do not implement encoding.TextMarshaler.
func (WrongText) MarshalText([]byte) ([]byte, error) { IgnoredCalls.Add(1); return nil, nil }

// A boolean result does not implement json.Unmarshaler.
func (*WrongInputJSON) UnmarshalJSON([]byte) bool { IgnoredCalls.Add(1); return true }

// A string parameter does not implement encoding.TextUnmarshaler.
func (*WrongInputText) UnmarshalText(string) error { IgnoredCalls.Add(1); return nil }

// Variadic methods do not match the no-argument marshaling interface.
func (Variadic) MarshalJSON(...byte) ([]byte, error) { IgnoredCalls.Add(1); return nil, nil }

// A defined byte-slice parameter is different from the interface's []byte parameter.
func (*WrongNamedInput) UnmarshalJSON(NamedBytes) error { IgnoredCalls.Add(1); return nil }

// A byte-slice alias retains the exact standard interface signature.
type Bytes = []byte
type OutputJSON int
type OutputText string
type InputJSON int
type InputText string
type Both int
type PointerJSON int

// Encode a JSON object instead of the underlying integer.
func (v OutputJSON) MarshalJSON() (Bytes, error) {
	JSONCalls.Add(1)
	return json.Marshal(struct{ Value int }{Value: int(v)})
}

// Encode labeled text instead of the underlying string.
func (v OutputText) MarshalText() ([]byte, error) {
	TextCalls.Add(1)
	return []byte("label:" + string(v)), nil
}

// Decode an actual JSON object into the named integer.
func (v *InputJSON) UnmarshalJSON(raw Bytes) error {
	JSONCalls.Add(1)
	var object struct{ Value int }
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	*v = InputJSON(object.Value)
	return nil
}

// Decode labeled text into the named string.
func (v *InputText) UnmarshalText(raw []byte) error {
	TextCalls.Add(1)
	if !strings.HasPrefix(string(raw), "label:") {
		return fmt.Errorf("missing label prefix")
	}
	*v = InputText(strings.TrimPrefix(string(raw), "label:"))
	return nil
}

// Prefer a JSON object when both output interfaces are implemented.
func (v Both) MarshalJSON() ([]byte, error) { return OutputJSON(v).MarshalJSON() }

// Keep a competing text interface that JSON must not select.
func (v Both) MarshalText() ([]byte, error) { return OutputText("unused").MarshalText() }

// Prefer the JSON object input interface over text input.
func (v *Both) UnmarshalJSON(raw []byte) error {
	var value InputJSON
	if err := value.UnmarshalJSON(raw); err != nil {
		return err
	}
	*v = Both(value)
	return nil
}

// Keep a competing input text interface that JSON must not select.
func (v *Both) UnmarshalText(raw []byte) error { var value InputText; return value.UnmarshalText(raw) }

// Pointer-only output participates only where the selected encoder can address the value.
func (v *PointerJSON) MarshalJSON() ([]byte, error) { return OutputJSON(*v).MarshalJSON() }

// Use quoted-field tags without overriding custom output methods.
type OutputFields struct {
	JSON OutputJSON `json:",string"`
	Text OutputText `json:",string"`
}

// Use quoted-field tags without overriding custom input methods.
type InputFields struct {
	JSON InputJSON `json:",string"`
	Text InputText `json:",string"`
}

// Expose the addressability-dependent combination for an explicit containing-type mapping.
type PointerFields struct {
	Value PointerJSON `json:",string"`
}
