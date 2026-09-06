// Use ordinary Go inputs and return values without a framework Context.
package consumer

// Input for creating a user.
type Request struct {
	// User name.
	// @openapi required nonnull minLength=2 examples=["Alice"]
	Name string
}

// Response after successful creation.
type Response struct {
	// User identifier.
	ID int64
	// User name.
	Name string
}

// Create a user
// Return the creation result for the independent frontend.
func Create(req Request) (Response, error) { return Response{ID: 1, Name: req.Name}, nil }

// Return ordinary text according to the input without a server framework.
func TextResult(left bool) string {
	if left {
		return "left"
	}
	return "right"
}

// Ordinary query input without a network framework.
type Search struct {
	// Search name.
	// @openapi required minLength=2
	Name string
	// Raw identifiers.
	IDs []byte
	// Optional limit.
	Limit *int
}

// Provide a neutral entry point with ordinary arguments and return values.
func Find(input Search) string { return input.Name }
