// Provide real JSON field-selection and omission samples for public projection tests.
package jsonfields

import "encoding/json"

// Keep a nonempty object representation even at its zero Go value.
type Inner struct{ Name string }

// Separate empty-value omission, zero-value omission, explicit null, and interface presence.
type Fields struct {
	Required       string
	Object         Inner             `json:",omitempty"`
	Array          [1]int            `json:",omitempty"`
	EmptyArray     [0]int            `json:",omitempty"`
	Slice          []string          `json:",omitempty"`
	Map            map[string]string `json:",omitempty"`
	Pointer        *int              `json:",omitempty"`
	PointerPresent *int
	Iface          any    `json:",omitempty"`
	ZeroObject     Inner  `json:",omitzero"`
	ZeroArray      [1]int `json:",omitzero"`
	Unicode        string `json:"名字"`
	Punctuation    string `json:"a/b.$:@![]{}()?<>"`
	Ignored        string `json:"-"`
}

// Preserve nil for a named pointer whose scalar value can be quoted.
type NumberPointer *int

// Apply string quoting only to the scalar kinds supported by the standard compatibility API.
type StringOptions struct {
	Array        [1]int          `json:",string"`
	Object       Inner           `json:",string"`
	Slice        []int           `json:",string"`
	Bytes        []byte          `json:",string"`
	Raw          json.RawMessage `json:",string"`
	Bool         bool            `json:",string"`
	Text         string          `json:",string"`
	Number       int             `json:",string"`
	Pointer      *int            `json:",string"`
	NamedPointer NumberPointer   `json:",string"`
}

// Keep unstable reserved-character tags isolated from ordinary fields.
type EscapedName struct {
	Value string `json:"bad\\name"`
}
type QuotedName struct {
	Value string `json:"'two,words'"`
}

// Exercise embedding depth, tag priority, optional pointer parents, and duplicate paths.
type Left struct {
	Collision string
	LeftOnly  int
}
type Right struct {
	Collision string
	RightOnly int
}
type Conflict struct {
	Left
	Right
}
type TaggedLeft struct {
	Value string `json:"Collision"`
}
type TaggedRight struct{ Collision int }
type Dominant struct {
	TaggedLeft
	TaggedRight
}
type PointerEmbedded struct {
	*Left
	Own int
}
type NamedEmbedded struct {
	Left `json:"payload"`
}
type Shadow struct {
	Left
	Collision bool
}
type Leaf struct{ Value string }
type ViaLeft struct{ Leaf }
type ViaRight struct{ Leaf }
type Repeated struct {
	ViaLeft
	ViaRight
}
type hidden struct{ Exposed string }
type UnexportedEmbedded struct{ hidden }
type Recursive struct {
	*Recursive
	Count int
}
