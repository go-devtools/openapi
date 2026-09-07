// Describe a tag-free contract independently of HTTP frameworks.
package api

// Information submitted when creating a user.
type Request struct {
	// User name.
	// @openapi required nonnull minLength=3 examples=["alice"]
	Name string
}

// Created user information.
type Response struct {
	// Assigned identifier.
	ID int64
	// Display name.
	Name string
}

// Create a user
//
// Return the assigned identifier and submitted name.
func Create(request Request) (Response, error) {
	return Response{ID: 1024, Name: request.Name}, nil
}
