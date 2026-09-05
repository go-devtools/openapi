package contracttest

import "testing"

// 原始 JSON 恰好达到字节预算时，内存样本计费不能重复计算分隔符。
// At the exact raw JSON byte limit, decoded sample accounting must not double-count separators.
func TestContractDecodedByteBoundary(t *testing.T) {
	for _, sample := range []string{`[true]`, `{"a":1}`, `[1,2]`, `"a"`} {
		limit := int64(len(sample))
		if limit < 4 {
			limit = 4
		}
		v, err := Compile([]byte(`true`), "", Options{MaxBytes: limit})
		if err != nil {
			t.Fatal(err)
		}
		if err = v.JSON([]byte(sample)); err != nil {
			t.Fatalf("%s：%v", sample, err)
		}
	}
}
