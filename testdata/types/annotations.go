package fixtures

// Strings cannot declare numeric bounds.

// 字符串不能声明数值边界。
type StringMinimum struct {
	// @openapi minimum=1
	Value string
}

// Check named field references against their actual string type.

// 命名类型的字段引用仍需按真实字符串检查。
type NamedMinimum struct {
	// @openapi minimum=1
	Value Role
}

// Locate conflicting length bounds at their source declaration.

// 长度上下界矛盾必须定位到源码声明。
type ReversedLength struct {
	// @openapi minLength=10 maxLength=2
	Value string
}

// Reject conflicting numeric bounds instead of generating an unsatisfiable business contract.

// 数值上下界矛盾不能生成不可满足的业务契约。
type ReversedNumber struct {
	// @openapi minimum=10 maximum=2
	Value int
}

// Do not allow annotations to override actual fixed-array lengths.

// 固定数组的真实长度不允许被声明覆盖。
type ChangedArray struct {
	// @openapi minItems=4
	Value [3]int
}

// Derive unsigned lower bounds from the actual encoding type.

// 无符号类型的数值下界来自真实编码类型。
type ChangedUnsigned struct {
	// @openapi minimum=-1
	Value uint
}

// Reject numeric values for boolean context directives.

// 布尔上下文指令不接受数字值。
type BadRequired struct {
	// @openapi required=5
	Value string
}

// Field annotations cannot fabricate the actual wire type.

// 字段说明不能伪造真实网络类型。
type ChangedType struct {
	// @openapi type="integer"
	Value string
}

// Actual response fields cannot be marked write-only.

// 实际响应字段不能标记为仅写。
type OutputWriteOnly struct {
	// @openapi writeOnly
	Value string
}

// Narrow accepted values with valid annotations and keep nullable idempotent for types already allowing null.

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

// Preserve explicit null values during typed decoding instead of replacing them with zero values.

// 显式空值不应在类型化解码时被变成零值。
type NullLength struct {
	// @openapi minLength=null
	Value string
}

// Do not silently ignore null values for boolean keywords.

// 布尔关键字的 null 不能被静默忽略。
type NullReadOnly struct {
	// @openapi readOnly=null
	Value string
}

// Require multiples to be strictly positive.

// 倍数必须严格大于零。
type ZeroMultiple struct {
	// @openapi multipleOf=0
	Value int
}

// Preserve conflicting large-integer bounds without floating-point rounding.

// 大整数范围不能因为浮点舍入而丢失矛盾。
type PreciseReversedNumber struct {
	// @openapi minimum=9007199254740993 maximum=9007199254740992
	Value int64
}

// Report budget errors for extreme annotation comparisons without allocating exponent-sized integers.

// 极端的声明比较明确报预算错误，不分配指数规模的整数。
type ExponentBudget struct {
	// @openapi minimum=1e999999999999999999 maximum=2e999999999999999999
	Value float64
}

// Allow open JSON types to declare constraints applying only to string branches.

// 开放 JSON 类型可以声明只针对字符串分支的约束。
type OpenConstraints struct {
	// @openapi minLength=2
	Value any
}

// Check named-type upper bounds together with field lower bounds.

// 命名类型上限与字段下限需要合并检查。
// @openapi maximum=2
type BoundedNumber int

// Reject sibling constraints that contradict the referenced component's own bounds.

// 组件引用上的兄弟约束不能与组件本身的边界矛盾。
type ReferencedRange struct {
	// @openapi minimum=3
	Value BoundedNumber
}
