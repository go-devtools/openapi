package validate

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

// Cross-check decimal and exponent integer classification using bounded exact rationals.
func FuzzSchemaNumberTraits(f *testing.F) {
	f.Add(int64(100), int16(-2), uint8(0))
	f.Add(int64(-1234), int16(3), uint8(2))
	f.Add(int64(0), int16(-127), uint8(0))
	f.Fuzz(func(t *testing.T, coefficient int64, exponent int16, position uint8) {
		text := strconv.FormatInt(coefficient, 10)
		sign := ""
		if strings.HasPrefix(text, "-") {
			sign = "-"
			text = text[1:]
		}
		index := int(position) % (len(text) + 1)
		if index > 0 && index < len(text) {
			text = text[:index] + "." + text[index:]
		}
		number := json.Number(sign + text + "e" + strconv.Itoa(int(exponent)%300))
		reference, ok := new(big.Rat).SetString(string(number))
		if !ok {
			t.Fatal("fixture generated an invalid number")
		}
		negative, zero, integer := numberTraits(number)
		if negative != (reference.Sign() < 0) || zero != (reference.Sign() == 0) || integer != reference.IsInt() {
			t.Fatalf("%s classification is %v/%v/%v, expected %s", number, negative, zero, integer, reference.RatString())
		}
	})
}
