// Exercise concrete generic helper calls without changing business wire types.
package helpergeneric

// Describe a payload whose field contract must survive helper instantiation.
type Item struct {
	// A display name with a declared minimum length.
	// @openapi minLength=3
	Name string
}

// Carry one actual generic payload.
type Envelope[T any] struct{ Data T }

// Keep multiple generic arguments distinct.
type Pair[A, B any] struct {
	First  A
	Second B
}

// Return the zero of the chosen concrete type.
func zero[T any]() T { var value T; return value }

// Initialize a named result with the chosen concrete zero value.
func namedZero[T any]() (value T) { return }

// Forward actual payloads into their instantiated named envelope.
func wrap[T any](value T) Envelope[T] { return Envelope[T]{Data: value} }

// Preserve both generic type arguments through explicit function-value instantiation.
func pair[A, B any](first A, second B) Pair[A, B] { return Pair[A, B]{First: first, Second: second} }

// Preserve concrete argument identity through a simple source helper.
func identity[T any](value T) T { return value }

// Preserve an outer instantiation in a returned closure.
func factory[T any](value T) func() Envelope[T] { return func() Envelope[T] { return wrap(value) } }

// Repeated source calls must substitute the caller's already concrete argument.
func nested[T any](value T) Envelope[Envelope[T]] { return wrap(wrap(value)) }

// Preserve declaration metadata on substituted anonymous fields.
func anonymous[T any](value T) any {
	return struct {
		// The actual selected payload is required by contract.
		// @openapi required
		Value T
		// Keep the original anonymous field's declared minimum.
		// @openapi minLength=3
		Label string
	}{Value: value, Label: "Ada"}
}

// Convert a constant to a concrete numeric argument, not the constraint interface.
func converted[T ~int64]() T { return T(1) }

// Preserve concrete type and constant arguments through bounded recursion.
func recursive[T any](value T, remaining int) Envelope[T] {
	if remaining == 0 {
		return wrap(value)
	}
	return recursive(value, remaining-1)
}

// Extract a payload from a concretely instantiated receiver.
func (value Envelope[T]) Extract() T { return value.Data }

// The analyzed method must retain constants from its actual source body.
func (value Envelope[T]) Status() int { return 207 }

// Keep direct instantiated zero values concrete.
func HZero() any { return zero[int]() }

// Keep named result initialization concrete.
func HNamedZero() any { return namedZero[int]() }

// Keep explicit generic type arguments.
func HExplicit() any { return wrap[Item](Item{Name: "Ada"}) }

// Keep inferred generic type arguments.
func HInferred() any { return wrap(Item{Name: "Ada"}) }

// Retain the actual function implementation through local function variables.
func HFunctionValue() any { f := identity[int]; return f(7) }

// Retain generic construction after a function value is assigned to another variable.
func HEnvelopeValue() any { f := wrap[Item]; alias := f; return alias(Item{Name: "Ada"}) }

// Support multiple explicit generic arguments in function values.
func HMultiple() any { f := pair[Item, int]; return f(Item{Name: "Ada"}, 7) }

// Retain generic lexical bindings inside returned closures.
func HClosure() any { f := factory(Item{Name: "Ada"}); return f() }

// Carry concrete payloads through nested helper calls.
func HNested() any { return nested(Item{Name: "Ada"}) }

// Preserve source comments on anonymous substituted fields.
func HAnonymous() any { return anonymous(Item{Name: "Ada"}) }

// Retain concrete conversions inside the helper body.
func HConverted() any { return converted[int64]() }

// Resolve method source using the original go/types function identity.
func HMethod() any { return (Envelope[Item]{Data: Item{Name: "Ada"}}).Extract() }

// Preserve receiver instantiation in method expressions.
func HMethodExpression() any {
	f := Envelope[Item].Extract
	return f(Envelope[Item]{Data: Item{Name: "Ada"}})
}

// Recursion remains finite under the shared depth budget.
func HRecursive() any { return recursive(Item{Name: "Ada"}, 3) }

// Capture emitted facts for separate runtime comparison with neutral compiler effects.
type Channel struct {
	Code int
	Body any
}

// Record the actual status and payload without a framework dependency.
func (c *Channel) Emit(code int, body any) { c.Code = code; c.Body = body }

// Forward status constants and actual DTO types into a response call.
func deliver[T any](c *Channel, code int, value T) { c.Emit(code, wrap(value)) }

// Emit a created response through a generic helper.
func HSend(c *Channel) { deliver(c, 201, Item{Name: "Ada"}) }

// A second instantiation must retain its own type and status.
func HSendOther(c *Channel) { deliver(c, 202, "ready") }

// Actual generic method bodies contribute response status facts.
func HMethodStatus(c *Channel) {
	value := Envelope[Item]{Data: Item{Name: "Ada"}}
	c.Emit(value.Status(), value.Extract())
}

// Preserve a scalar declaration on a field whose type must be substituted.
func fieldContract[T ~string](value T) any {
	return struct {
		// A declared string contract survives anonymous-field substitution.
		// @openapi minLength=3
		Value T
	}{Value: value}
}

// Instantiate a declaration on an anonymous generic scalar field.
func HFieldContract() any { return fieldContract("Ada") }

// Preserve typed nil pointers through concrete zero construction and interface boxing.
func HNilPointer() any { return zero[*Item]() }

// Preserve nil slices instead of inventing an empty array.
func HNilSlice() any { return zero[[]Item]() }

// Preserve nil maps instead of inventing an empty object.
func HNilMap() any { return zero[map[string]Item]() }

// A fixed empty array is distinct from a nil slice.
func HEmptyArray() any { return zero[[0]Item]() }

// Separate instantiations within one handler must not overwrite each other's types.
func HBranches(flag bool) any {
	if flag {
		return wrap(Item{Name: "Ada"})
	}
	return wrap("ready")
}

// Keep generic alias metadata while substituting its named target.
type Alias[T any] = Envelope[T]

// Return the explicitly instantiated alias.
func aliasValue[T any](value T) Alias[T] { return Alias[T]{Data: value} }

// Project an alias constructed inside a generic source helper.
func HAlias() any { return aliasValue(Item{Name: "Ada"}) }

// Substitute nested pointer, array, slice, and map members without changing their wire representation.
func collections[T any](value T) any {
	return struct {
		Pointer *T
		Array   [1]T
		Slice   []T
		Map     map[string]T
	}{
		Pointer: &value, Array: [1]T{value}, Slice: []T{value}, Map: map[string]T{"first": value},
	}
}

// Preserve concrete generic members in anonymous collection payloads.
func HCollections() any { return collections(Item{Name: "Ada"}) }

// Analyze pointer receiver methods with their own concrete receiver parameters.
func (value *Envelope[T]) Deliver(c *Channel) { c.Emit(206, value.Data) }

// Carry source effects through an automatically addressed generic receiver.
func HPointerMethodStatus(c *Channel) {
	value := Envelope[Item]{Data: Item{Name: "Ada"}}
	value.Deliver(c)
}
