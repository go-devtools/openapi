package fixtures

// 字符串不能声明数值边界。
type StringMinimum struct {
	// @openapi minimum=1
	Value string
}

// 命名类型的字段引用仍需按真实字符串检查。
type NamedMinimum struct {
	// @openapi minimum=1
	Value Role
}

// 长度上下界矛盾必须定位到源码声明。
type ReversedLength struct {
	// @openapi minLength=10 maxLength=2
	Value string
}

// 数值上下界矛盾不能生成不可满足的业务契约。
type ReversedNumber struct {
	// @openapi minimum=10 maximum=2
	Value int
}

// 固定数组的真实长度不允许被声明覆盖。
type ChangedArray struct {
	// @openapi minItems=4
	Value [3]int
}

// 无符号类型的数值下界来自真实编码类型。
type ChangedUnsigned struct {
	// @openapi minimum=-1
	Value uint
}

// 布尔上下文指令不接受数字值。
type BadRequired struct {
	// @openapi required=5
	Value string
}

// 字段说明不能伪造真实网络类型。
type ChangedType struct {
	// @openapi type="integer"
	Value string
}

// 实际响应字段不能标记为仅写。
type OutputWriteOnly struct {
	// @openapi writeOnly
	Value string
}

// 合法声明缩小可接受值域，nullable 对已经允许 null 的类型保持幂等。
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

// 显式空值不应在类型化解码时被变成零值。
type NullLength struct {
	// @openapi minLength=null
	Value string
}

// 布尔关键字的 null 不能被静默忽略。
type NullReadOnly struct {
	// @openapi readOnly=null
	Value string
}

// 倍数必须严格大于零。
type ZeroMultiple struct {
	// @openapi multipleOf=0
	Value int
}

// 大整数范围不能因为浮点舍入而丢失矛盾。
type PreciseReversedNumber struct {
	// @openapi minimum=9007199254740993 maximum=9007199254740992
	Value int64
}

// 极端的声明比较明确报预算错误，不分配指数规模的整数。
type ExponentBudget struct {
	// @openapi minimum=1e999999999999999999 maximum=2e999999999999999999
	Value float64
}

// 开放 JSON 类型可以声明只针对字符串分支的约束。
type OpenConstraints struct {
	// @openapi minLength=2
	Value any
}

// 命名类型上限与字段下限需要合并检查。
// @openapi maximum=2
type BoundedNumber int

// 组件引用上的兄弟约束不能与组件本身的边界矛盾。
type ReferencedRange struct {
	// @openapi minimum=3
	Value BoundedNumber
}
