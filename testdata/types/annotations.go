package fixtures

// Strings cannot declare numeric bounds.
type StringMinimum struct {
	// @openapi minimum=1
	Value string
}

// Check named field references against their actual string type.
type NamedMinimum struct {
	// @openapi minimum=1
	Value Role
}

// Locate conflicting length bounds at their source declaration.
type ReversedLength struct {
	// @openapi minLength=10 maxLength=2
	Value string
}

// Reject conflicting numeric bounds instead of generating an unsatisfiable business contract.
type ReversedNumber struct {
	// @openapi minimum=10 maximum=2
	Value int
}

// Do not allow annotations to override actual fixed-array lengths.
type ChangedArray struct {
	// @openapi minItems=4
	Value [3]int
}

// Derive unsigned lower bounds from the actual encoding type.
type ChangedUnsigned struct {
	// @openapi minimum=-1
	Value uint
}

// Reject numeric values for boolean context directives.
type BadRequired struct {
	// @openapi required=5
	Value string
}

// Field annotations cannot fabricate the actual wire type.
type ChangedType struct {
	// @openapi type="integer"
	Value string
}

// Actual response fields cannot be marked write-only.
type OutputWriteOnly struct {
	// @openapi writeOnly
	Value string
}

// Narrow accepted values with valid annotations and keep nullable idempotent for types already allowing null.
type ValidConstraints struct {
	// @openapi minimum=0 maximum=10
	Count uint
	// @openapi minLength=2 maxLength=10
	Name string
	// @openapi nullable
	Note *string
	// @openapi minItems=3 maxItems=3
	Coordinates [3]int
}

// Preserve explicit null values during typed decoding instead of replacing them with zero values.
type NullLength struct {
	// @openapi minLength=null
	Value string
}

// Do not silently ignore null values for boolean keywords.
type NullReadOnly struct {
	// @openapi readOnly=null
	Value string
}

// Require multiples to be strictly positive.
type ZeroMultiple struct {
	// @openapi multipleOf=0
	Value int
}

// Preserve conflicting large-integer bounds without floating-point rounding.
type PreciseReversedNumber struct {
	// @openapi minimum=9007199254740993 maximum=9007199254740992
	Value int64
}

// Report budget errors for extreme annotation comparisons without allocating exponent-sized integers.
type ExponentBudget struct {
	// @openapi minimum=1e999999999999999999 maximum=2e999999999999999999
	Value float64
}

// Allow open JSON types to declare constraints applying only to string branches.
type OpenConstraints struct {
	// @openapi minLength=2
	Value any
}

// Check named-type upper bounds together with field lower bounds.
// @openapi maximum=2
type BoundedNumber int

// Reject sibling constraints that contradict the referenced component's own bounds.
type ReferencedRange struct {
	// @openapi minimum=3
	Value BoundedNumber
}
