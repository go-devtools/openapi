package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verify source-based Schema export and validation of actual CLI input.
func TestSchemaAndCheckCommands(t *testing.T) {
	output := filepath.Join(t.TempDir(), "schema.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"schema", "--dir", "../../testdata/types", "--type", "Request", "--projection", "request", "--output", output}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("generation failed: %d %s", code, stderr.String())
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "#/$defs/") {
		t.Fatal("standalone Schema reference is missing")
	}
	specFile := filepath.Join(t.TempDir(), "openapi.json")
	if err = os.WriteFile(specFile, []byte(`{"openapi":"3.1.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if run(context.Background(), []string{"check", "--spec", specFile}, &stdout, &stderr) == 0 {
		t.Fatal("incompatible specification was accepted")
	}
}
