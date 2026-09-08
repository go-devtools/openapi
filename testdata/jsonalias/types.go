// Provide aliases with distinct use-site declarations but unchanged Go wire types.
package jsonalias

// A declared display label needs at least three characters.
// @openapi minLength=3
type Label = string

// Alias chains add constraints rather than relaxing the target contract.
// @openapi maxLength=5
type Short = Label

// A weaker declaration cannot erase an inherited stronger bound.
// @openapi minLength=1
type Relaxed = Label

// Incompatible inherited bounds must be reported at the alias declaration.
// @openapi maxLength=2
type Impossible = Label

// An explicit alias enum has an unambiguous declared value set.
// @openapi enum=["red","blue"]
type Color = string

// A bare enum flag cannot identify a separate Go constant type for an alias.
// @openapi enum
type AmbiguousEnum = string

// Instantiate generic alias constraints against their actual element types.
// @openapi minItems=1
type Batch[T any] = []T

// Retain an unconstrained target for shared-reference isolation checks.
type Base struct{ Value string }

// Constrain only this use of the shared target.
// @openapi minProperties=1
type NamedAlias = Base

// Refer through an alias without expanding a recursive graph indefinitely.
type Node struct {
	Value string
	Next  *NodeAlias
}

// The declaration applies at each explicit alias use.
// @openapi minProperties=1
type NodeAlias = Node

// Plain target fields must not inherit restrictions from neighboring alias fields.
type Pair struct {
	Restricted NamedAlias
	Plain      Base
	Label      Label
	Text       string
}
