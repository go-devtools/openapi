// 提供真实源码投影的零 tag 类型。
package fixtures

import (
	"encoding/json"
	"time"
)

// 创建用户时提交的信息。
type Request struct {
	// 用户名。
	// @openapi required nonnull minLength=3 examples=["alice"]
	Name string
	// 可选上级。
	Parent *Request
	// 原始字节。
	Data []byte
	// 按编号索引的标签。
	Labels map[int]string
	// 创建时间。
	Created time.Time
	// 开放 JSON 值。
	Raw json.RawMessage
}

// 通用分页结果。
type Page[T any] struct {
	Items []T
	Total int64
}

// 用户角色。
// @openapi enum
type Role string

// 只枚举显式封闭类型的常量。
const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// 自定义编码不得被执行以探测结构。
type Custom struct{ Hidden string }

// 表示只有真实运行时才会执行的用户序列化行为。
func (Custom) MarshalJSON() ([]byte, error) { panic("生成器不允许执行用户代码") }
