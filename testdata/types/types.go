// Provide real tag-free source types for projection tests.

// 提供真实源码投影的零 tag 类型。
package fixtures

import (
	"encoding/json"
	"time"
)

// Information submitted when creating a user.

// 创建用户时提交的信息。
type Request struct {
	// User name.

	// 用户名。
	// @openapi required nonnull minLength=3 examples=["alice"]
	Name string
	// Optional parent.

	// 可选上级。
	Parent *Request
	// Raw bytes.

	// 原始字节。
	Data []byte
	// Labels indexed by numeric identifiers.

	// 按编号索引的标签。
	Labels map[int]string
	// Creation time.

	// 创建时间。
	Created time.Time
	// An unconstrained JSON value.

	// 开放 JSON 值。
	Raw json.RawMessage
}

// Generic paginated results.

// 通用分页结果。
type Page[T any] struct {
	Items []T
	Total int64
}

// User role.

// 用户角色。
// @openapi enum
type Role string

// Enumerate constants only for explicitly closed types.

// 只枚举显式封闭类型的常量。
const (
	// Regular user

	// 普通用户
	RoleUser Role = "user"
	// Administrator

	// 管理员
	RoleAdmin Role = "admin"
)

// Never execute custom encoding methods to discover their shape.

// 自定义编码不得被执行以探测结构。
type Custom struct{ Hidden string }

// Represent serialization that must execute only in the real application.

// 表示只有真实运行时才会执行的用户序列化行为。
func (Custom) MarshalJSON() ([]byte, error) { panic("生成器不允许执行用户代码") }

// Cover iota and standalone constant comments in the state enumeration.

// 状态枚举覆盖 iota 和单独声明的常量注释。
// @openapi enum
type State int

// Ordered state values.

// 有序状态值。
const (
	// Pending

	// 待处理
	Pending State = iota
	// Running

	// 执行中
	Running
)

// Completed

// 已完成
const Done State = 2

// Do not duplicate wire enum values for source aliases of the same state.

// 相同状态的源码别名不重复生成网络枚举。
const RunningAlias State = Running

// Encode floating-point enums as actual numbers rather than Go rational text.

// 浮点枚举使用真实浮点编码，不能输出 Go 的有理数文本。
// @openapi enum
type Fraction float64

// Example ratios with finite precision.

// 有限精度的示例比例。
const (
	// One half

	// 一半
	FractionHalf Fraction = 1.0 / 2
	// Two thirds

	// 三分之二
	FractionTwoThirds Fraction = 2.0 / 3
)
