// Use ordinary Go inputs and return values without a framework Context.

// 此业务包仅使用普通 Go 输入与返回值，没有框架 Context。
package consumer

// Input for creating a user.

// 创建用户的输入。
type Request struct {
	// User name.

	// 用户名。
	// @openapi required nonnull minLength=2 examples=["小明"]
	Name string
}

// Response after successful creation.

// 创建完成后的响应。
type Response struct {
	// User identifier.

	// 用户编号。
	ID int64
	// User name.

	// 用户名。
	Name string
}

// Create a user
// Return the creation result for the independent frontend.

// 创建用户
//
// 返回独立前端的创建结果。
func Create(req Request) (Response, error) { return Response{ID: 1, Name: req.Name}, nil }

// 按输入返回普通文本，不依赖服务器框架。
// Return ordinary text according to the input without a server framework.
func TextResult(left bool) string {
	if left {
		return "left"
	}
	return "right"
}

// 普通查询输入，不依赖任何网络框架。
// Ordinary query input without a network framework.
type Search struct {
	// 查询名称。 Search name.
	// @openapi required minLength=2
	Name string
	// 原始编号。 Raw identifiers.
	IDs []byte
	// 可选限制。 Optional limit.
	Limit *int
}

// 使用普通参数和返回值提供中立入口。
// Provide a neutral entry point with ordinary arguments and return values.
func Find(input Search) string { return input.Name }
