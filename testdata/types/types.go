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
	// 普通用户
	RoleUser Role = "user"
	// 管理员
	RoleAdmin Role = "admin"
)

// 自定义编码不得被执行以探测结构。
type Custom struct{ Hidden string }

// 表示只有真实运行时才会执行的用户序列化行为。
func (Custom) MarshalJSON() ([]byte, error) { panic("生成器不允许执行用户代码") }

// 状态枚举覆盖 iota 和单独声明的常量注释。
// @openapi enum
type State int

// 有序状态值。
const (
	// 待处理
	Pending State = iota
	// 执行中
	Running
)

// 已完成
const Done State = 2

// 相同状态的源码别名不重复生成网络枚举。
const RunningAlias State = Running

// 浮点枚举使用真实浮点编码，不能输出 Go 的有理数文本。
// @openapi enum
type Fraction float64

// 有限精度的示例比例。
const (
	// 一半
	FractionHalf Fraction = 1.0 / 2
	// 三分之二
	FractionTwoThirds Fraction = 2.0 / 3
)
