package contracttest

import (
	"encoding/json"
	"os"
	"testing"
)

// Validate the same raw JSON examples using the independent engine, official meta-schema, and local checker.
func TestOfficialSchemaKeywordMatrix(t *testing.T) {
	validator := official32(t)
	raw, err := os.ReadFile("../testdata/golden/schema-keywords.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Schema json.RawMessage
		Valid  bool
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			doc, err := decode([]byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"paths":{},"components":{"schemas":{"X":` + string(tc.Schema) + `}}}`))
			if err != nil {
				t.Fatal(err)
			}
			err = validator.Validate(doc)
			if (err == nil) != tc.Valid {
				t.Fatalf("valid=%v, independent validation=%v", tc.Valid, err)
			}
		})
	}
}
