package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CLI 必须转发索引预算，超限返回非零并保留稳定诊断编码。
// The CLI forwards index budgets and returns a nonzero status with the stable diagnostic code.
func TestCheckIndexBudgetFlag(t *testing.T) {
	file := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(file, []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, size := range []string{"1", "4096"} {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), []string{"check", "--spec", file, "--max-index-bytes", size}, &stdout, &stderr)
		if size == "1" {
			if code == 0 || !strings.Contains(stdout.String(), "openapi.spec.budget") {
				t.Fatal(code, stdout.String(), stderr.String())
			}
		} else if code != 0 {
			t.Fatal(code, stdout.String(), stderr.String())
		}
	}
}
