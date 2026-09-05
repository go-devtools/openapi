// 此业务包仅使用普通 Go 输入与返回值，没有框架 Context。
package consumer

// 创建用户的输入。
type Request struct {
	// 用户名。
	// @openapi required nonnull minLength=2 examples=["小明"]
	Name string
}

// 创建完成后的响应。
type Response struct {
	// 用户编号。
	ID int64
	// 用户名。
	Name string
}

// 创建用户
//
// 返回独立前端的创建结果。
func Create(req Request) (Response, error) { return Response{ID: 1, Name: req.Name}, nil }
