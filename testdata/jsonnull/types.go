// Exercise null declarations and annotations hidden by opaque byte encoding.
package jsonnull

import "encoding/json"

// Retain a recursive nullable member inside a nonnull use site.
type Node struct {
	Value string
	Next  *Node
}

// A named nullable collection is referenced rather than inlined at its use site.
type Names []string

// Distinguish presence and nullability without changing decoder behavior.
type Request struct {
	// @openapi nonnull
	Child *Node
	// @openapi nonnull
	Anything any
	// @openapi nonnull
	Raw json.RawMessage
	// @openapi nonnull
	Values Names
	// @openapi nonnull
	Count *int
	// @openapi required nonnull
	Must          *Node
	AllowedChild  *Node
	AllowedValues Names
	// @openapi nonnull=false
	Unrestricted *Node
}

// Supply custom union schemas through a centralized mapper.
type Custom int
type Mapped struct {
	// @openapi nonnull
	Value Custom
}

// Type checks must still refer to the actual mapped numeric wire type.
type WrongLength struct {
	// @openapi nonnull minLength=1
	Value Custom
}

// Both declarations cannot be enabled together.
type Conflict struct {
	// @openapi nonnull nullable
	Value *Node
}

// Plain documentation on a byte type does not declare a scalar constraint.
type Plain uint8
type PlainBytes []Plain

// Closed byte values cannot silently disappear into a Base64 string projection.
// @openapi enum
type Flag uint8

// Enumerate the declared scalar byte values.
const (
	First  Flag = 1
	Second Flag = 2
)

type FlagBytes []Flag

// Alias declarations also require an explicit representation when their leaf is opaque.
// @openapi minimum=1
type AnnotatedAlias = byte
type AliasBytes []AnnotatedAlias

// Fixed arrays expose scalar elements and therefore preserve ordinary element constraints.
type FlagArray [2]Flag
