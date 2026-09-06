package contracttest

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// Pins official resource bytes in the optional package to prevent unrecorded upstream replacements.
func TestBuiltinDialectChecksums(t *testing.T) {
	expected := map[string]string{
		"oas31-dialect.json": "8a0e89e365dadbebce2921ce6244340c1090e9d544c60d977e9ad6b97a61227b",
		"oas31-meta.json":    "267a88226e64e96dfc8c89dbd7e863160c84715e0fb893ca1d9fbf9f830f1f54",
		"oas32-dialect.json": "4e2c989f3d1e6489d41bc1ca4ade11743278c612b82fba9144c6116c79f1c273",
		"oas32-meta.json":    "a1959c0aa1f9a7ce58f2b75699be5bc5187c8a25901fe04a227f1d9419ba4e9c",
	}
	for name, want := range expected {
		raw, err := builtinDialects.ReadFile("dialects/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != want {
			t.Fatalf("%s: %s != %s", name, got, want)
		}
	}
}
