package compiler

import (
	"go/constant"
	"go/token"
	"go/types"
	"math/big"
)

// Propagate increments/decrements at the actual Go integer width, retaining types without invented constants for unknown values or float rounding.
// 按实际 Go 整数位宽传播自增/自减，未知值或浮点舍入保留类型而不伪造常量。
func incrementValue(value Value, target types.Type, sizes types.Sizes, operation token.Token) Value {
	result := Value{Type: target}
	if value.Constant == nil || target == nil || sizes == nil {
		return result
	}
	basic, ok := target.Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsInteger == 0 {
		return result
	}
	integer, ok := new(big.Int).SetString(value.Constant.ExactString(), 10)
	if !ok {
		return result
	}
	if operation == token.INC {
		integer.Add(integer, big.NewInt(1))
	} else {
		integer.Sub(integer, big.NewInt(1))
	}
	bits := sizes.Sizeof(target) * 8
	if bits < 1 || bits > 64 {
		return result
	}
	modulus := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	integer.Mod(integer, modulus)
	if basic.Info()&types.IsUnsigned == 0 && integer.Bit(int(bits)-1) != 0 {
		integer.Sub(integer, modulus)
	}
	result.Constant = constant.Make(integer)
	return result
}
