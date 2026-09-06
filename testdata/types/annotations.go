package fixtures

// 字符串不能声明数值边界。

// Strings cannot declare numeric bounds.
type StringMinimum struct {
	// @openapi minimum=1
	Value string
}

// 命名类型的字段引用仍需按真实字符串检查。

// Check named field references against their actual string type.
type NamedMinimum struct {
	// @openapi minimum=1
	Value Role
}

// 长度上下界矛盾必须定位到源码声明。

// Locate conflicting length bounds at their source declaration.
type ReversedLength struct {
	// @openapi minLength=10 maxLength=2
	Value string
}

// 数值上下界矛盾不能生成不可满足的业务契约。

// Reject conflicting numeric bounds instead of generating an unsatisfiable business contract.
type ReversedNumber struct {
	// @openapi minimum=10 maximum=2
	Value int
}

// 固定数组的真实长度不允许被声明覆盖。

// Do not allow annotations to override actual fixed-array lengths.
type ChangedArray struct {
	// @openapi minItems=4
	Value [3]int
}

// 无符号类型的数值下界来自真实编码类型。

// Derive unsigned lower bounds from the actual encoding type.
type ChangedUnsigned struct {
	// @openapi minimum=-1
	Value uint
}

// 布尔上下文指令不接受数字值。

// Reject numeric values for boolean context directives.
type BadRequired struct {
	// @openapi required=5
	Value string
}

// 字段说明不能伪造真实网络类型。

// Field annotations cannot fabricate the actual wire type.
type ChangedType struct {
	// @openapi type="integer"
	Value string
}

// 实际响应字段不能标记为仅写。

// Actual response fields cannot be marked write-only.
type OutputWriteOnly struct {
	// @openapi writeOnly
	Value string
}

// 合法声明缩小可接受值域，nullable 对已经允许 null 的类型保持幂等。

// Narrow accepted values with valid annotations and keep nullable idempotent for types already allowing null.
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

// Preserve explicit null values during typed decoding instead of replacing them with zero values.
type NullLength struct {
	// @openapi minLength=null
	Value string
}

// 布尔关键字的 null 不能被静默忽略。

// Do not silently ignore null values for boolean keywords.
type NullReadOnly struct {
	// @openapi readOnly=null
	Value string
}

// 倍数必须严格大于零。

// Require multiples to be strictly positive.
type ZeroMultiple struct {
	// @openapi multipleOf=0
	Value int
}

// 大整数范围不能因为浮点舍入而丢失矛盾。

// Preserve conflicting large-integer bounds without floating-point rounding.
type PreciseReversedNumber struct {
	// @openapi minimum=9007199254740993 maximum=9007199254740992
	Value int64
}

// 极端的声明比较明确报预算错误，不分配指数规模的整数。

// Report budget errors for extreme annotation comparisons without allocating exponent-sized integers.
type ExponentBudget struct {
	// @openapi minimum=1e999999999999999999 maximum=2e999999999999999999
	Value float64
}

// 开放 JSON 类型可以声明只针对字符串分支的约束。

// Allow open JSON types to declare constraints applying only to string branches.
type OpenConstraints struct {
	// @openapi minLength=2
	Value any
}

// 命名类型上限与字段下限需要合并检查。

// Check named-type upper bounds together with field lower bounds.
// @openapi maximum=2
type BoundedNumber int

// 组件引用上的兄弟约束不能与组件本身的边界矛盾。

// Reject sibling constraints that contradict the referenced component's own bounds.
type ReferencedRange struct {
	// @openapi minimum=3
	Value BoundedNumber
}
