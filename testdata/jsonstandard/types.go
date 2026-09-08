// Provide standard-library wire values and methods active in the required JSON implementation.
package jsonstandard

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"sync/atomic"
	"time"
)

// Keep ordinary byte names and aliases separate from custom byte codecs.
type Byte uint8
type AliasByte = byte
type NamedBytes []Byte
type InputByte uint8
type InputBytes []InputByte

// Count only actual runtime codec calls.
var Calls atomic.Int64

// Input byte arrays may call the method, while Base64 input bypasses element methods.
func (v *InputByte) UnmarshalJSON(raw []byte) error {
	Calls.Add(1)
	var value uint8
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*v = InputByte(value)
	return nil
}

// Cover standard-library representations without DTO annotations.
type Standard struct {
	Timestamp time.Time
	Duration  time.Duration
	Number    json.Number
	Unsigned  uint64
	Signed    int64
	Bytes     []byte
	Named     NamedBytes
	Alias     []AliasByte
	Array     [2]Byte
	Raw       json.RawMessage
	Any       any
}

// Preserve options ignored by the standard compatibility API as ordinary nested properties.
type IgnoredOptions struct {
	Inline  map[string]int `json:",inline"`
	Unknown map[string]int `json:",unknown"`
}

// These format options fail at runtime in the selected standard compatibility configuration.
type HexFormat struct {
	Data []byte `json:",format:hex"`
}
type ArrayFormat struct {
	Data []byte `json:",format:array"`
}
type DurationFormat struct {
	Value time.Duration `json:",format:units"`
}
type TimeFormat struct {
	Value time.Time `json:",format:unix"`
}

// Add interfaces that the required standard JSON implementation actually calls.
type AppendByte uint8
type StreamByte uint8
type StreamInput int
type AppendSlice []AppendByte
type StreamSlice []StreamByte
type AppendMap map[AppendByte]string
type StreamMap map[StreamByte]string
type StreamInputMap map[StreamInput]string
type EncoderAlias = jsontext.Encoder

// Append labeled text while respecting the supplied output prefix.
func (v AppendByte) AppendText(out []byte) ([]byte, error) {
	Calls.Add(1)
	return append(out, fmt.Sprintf("byte:%d", v)...), nil
}

// Encode labeled text through the active streaming interface, including an encoder alias.
func (v StreamByte) MarshalJSONTo(enc *EncoderAlias) error {
	Calls.Add(1)
	return enc.WriteToken(jsontext.String(fmt.Sprintf("byte:%d", v)))
}

// Decode a real JSON object through the active streaming input interface.
func (v *StreamInput) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	Calls.Add(1)
	raw, err := dec.ReadValue()
	if err != nil {
		return err
	}
	var object struct{ Value int }
	if err = json.Unmarshal(raw, &object); err != nil {
		return err
	}
	*v = StreamInput(object.Value)
	return nil
}

// Keep same-name methods with incompatible interfaces from changing ordinary integer projection.
type WrongStream int
type WrongAppend int

// A bytes.Buffer parameter is not a jsontext.Encoder parameter.
func (WrongStream) MarshalJSONTo(*bytes.Buffer) error { Calls.Add(1); return nil }

// A string parameter is not the TextAppender byte-slice parameter.
func (WrongAppend) AppendText(string) ([]byte, error) { Calls.Add(1); return nil, nil }

// Custom streaming and append methods still own string-tagged field representations.
type QuotedExtended struct {
	Stream StreamByte `json:",string"`
	Append AppendByte `json:",string"`
}
type QuotedStreamInput struct {
	Value StreamInput `json:",string"`
}

// Keep a competing legacy text method to verify appender precedence.
func (AppendByte) MarshalText() ([]byte, error) { Calls.Add(1); return []byte("legacy-text"), nil }

// Keep a competing legacy JSON method to verify streaming output precedence.
func (StreamByte) MarshalJSON() ([]byte, error) { Calls.Add(1); return []byte(`"legacy-json"`), nil }

// Streaming input must take precedence over this legacy method.
func (*StreamInput) UnmarshalJSON([]byte) error {
	Calls.Add(1)
	return fmt.Errorf("legacy input must not run")
}
