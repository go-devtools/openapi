// Provide real tag-free source types for projection tests.
package fixtures

import (
	"encoding/json"
	"time"
)

// Information submitted when creating a user.
type Request struct {

	// User name.
	// @openapi required nonnull minLength=3 examples=["alice"]
	Name string

	// Optional parent.
	Parent *Request

	// Raw bytes.
	Data []byte

	// Labels indexed by numeric identifiers.
	Labels map[int]string

	// Creation time.
	Created time.Time

	// An unconstrained JSON value.
	Raw json.RawMessage
}

// Generic paginated results.
type Page[T any] struct {
	Items []T
	Total int64
}

// User role.
// @openapi enum
type Role string

// Enumerate constants only for explicitly closed types.
const (

	// Regular user
	RoleUser Role = "user"

	// Administrator
	RoleAdmin Role = "admin"
)

// Never execute custom encoding methods to discover their shape.
type Custom struct{ Hidden string }

// Represent serialization that must execute only in the real application.
func (Custom) MarshalJSON() ([]byte, error) { panic("generator must not execute user code") }

// Cover iota and standalone constant comments in the state enumeration.
// @openapi enum
type State int

// Ordered state values.
const (

	// Pending
	Pending State = iota

	// Running
	Running
)

// Completed
const Done State = 2

// Do not duplicate wire enum values for source aliases of the same state.
const RunningAlias State = Running

// Encode floating-point enums as actual numbers rather than Go rational text.
// @openapi enum
type Fraction float64

// Example ratios with finite precision.
const (

	// One half
	FractionHalf Fraction = 1.0 / 2

	// Two thirds
	FractionTwoThirds Fraction = 2.0 / 3
)
