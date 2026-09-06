// 提供真实源码投影的零 tag 类型。

// Provide real tag-free source types for projection tests.
package fixtures

import (
	"encoding/json"
	"time"
)

// 创建用户时提交的信息。

// Information submitted when creating a user.
type Request struct {
	// 用户名。

	// User name.
	// @openapi required nonnull minLength=3 examples=["alice"]
	Name string
	// 可选上级。

	// Optional parent.
	Parent *Request
	// 原始字节。

	// Raw bytes.
	Data []byte
	// 按编号索引的标签。

	// Labels indexed by numeric identifiers.
	Labels map[int]string
	// 创建时间。

	// Creation time.
	Created time.Time
	// 开放 JSON 值。

	// An unconstrained JSON value.
	Raw json.RawMessage
}

// 通用分页结果。

// Generic paginated results.
type Page[T any] struct {
	Items []T
	Total int64
}

// 用户角色。

// User role.
// @openapi enum
type Role string

// 只枚举显式封闭类型的常量。

// Enumerate constants only for explicitly closed types.
const (
	// 普通用户

	// Regular user
	RoleUser Role = "user"
	// 管理员

	// Administrator
	RoleAdmin Role = "admin"
)

// 自定义编码不得被执行以探测结构。

// Never execute custom encoding methods to discover their shape.
type Custom struct{ Hidden string }

// 表示只有真实运行时才会执行的用户序列化行为。

// Represent serialization that must execute only in the real application.
func (Custom) MarshalJSON() ([]byte, error) { panic("generator must not execute user code") }

// 状态枚举覆盖 iota 和单独声明的常量注释。

// Cover iota and standalone constant comments in the state enumeration.
// @openapi enum
type State int

// 有序状态值。

// Ordered state values.
const (
	// 待处理

	// Pending
	Pending State = iota
	// 执行中

	// Running
	Running
)

// 已完成

// Completed
const Done State = 2

// 相同状态的源码别名不重复生成网络枚举。

// Do not duplicate wire enum values for source aliases of the same state.
const RunningAlias State = Running

// 浮点枚举使用真实浮点编码，不能输出 Go 的有理数文本。

// Encode floating-point enums as actual numbers rather than Go rational text.
// @openapi enum
type Fraction float64

// 有限精度的示例比例。

// Example ratios with finite precision.
const (
	// 一半

	// One half
	FractionHalf Fraction = 1.0 / 2
	// 三分之二

	// Two thirds
	FractionTwoThirds Fraction = 2.0 / 3
)
