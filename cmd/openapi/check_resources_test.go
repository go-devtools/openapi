package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Read resources from the explicit manifest and resolve relative paths against its directory.
// 命令通过用户指定清单读取资源；清单相对路径相对于清单目录解析。
func TestCheckResourceManifest(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"openapi.json":   `{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"schemas":{"A":{"$ref":"schema.json"}},"examples":{"E":{"externalValue":"example.txt"}}}}`,
		"schema.json":    `{"type":"string"}`,
		"example.txt":    "plain text",
		"resources.json": `[{"uri":"https://example.test/schema.json","file":"schema.json"},{"uri":"https://example.test/example.txt","file":"example.txt","kind":"example"}]`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"check", "--spec", filepath.Join(dir, "openapi.json"), "--base-uri", "https://example.test/openapi.json", "--resources", filepath.Join(dir, "resources.json")}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("explicit resource check failed %d: %s %s", code, stdout.String(), stderr.String())
	}
	var report map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if d, ok := report["diagnostics"].([]any); !ok || len(d) != 0 {
		t.Fatal(report)
	}
	for _, extra := range [][]string{{"--max-resources", "2"}, {"--max-references", "1"}, {"--max-bytes", "16"}} {
		stdout.Reset()
		stderr.Reset()
		if run(context.Background(), append(append([]string{}, args...), extra...), &stdout, &stderr) == 0 {
			t.Fatalf("command ignored budget %v", extra)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout.Reset()
	stderr.Reset()
	if run(ctx, args, &stdout, &stderr) == 0 || !strings.Contains(stderr.String(), "canceled") {
		t.Fatal("canceled command still executed", stderr.String())
	}
}

// Reject invalid manifests and missing resources while allowing help to exit successfully.
// 错误清单和未提供的资源必须明确失败，help 应正常退出。
func TestCheckManifestErrorsAndHelp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "openapi.json")
	os.WriteFile(file, []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"}}`), 0600)
	manifest := filepath.Join(dir, "resources.json")
	for _, data := range []string{
		`[{"uri":"https://example.test/a","file":"missing.json"}]`,
		`[{"uri":"https://example.test/a","file":"openapi.json","kind":"remote"}]`,
		`[{"uri":"https://example.test/a","file":"openapi.json"},{"uri":"https://example.test/a","file":"openapi.json"}]`,
		`[{"uri":"https://example.test/a","file":"openapi.json","download":true}]`,
		`[] false`,
	} {
		if err := os.WriteFile(manifest, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		var out, errout bytes.Buffer
		if run(context.Background(), []string{"check", "--spec", file, "--resources", manifest}, &out, &errout) == 0 {
			t.Fatal("invalid manifest was accepted", data)
		}
	}
	var out, errout bytes.Buffer
	if run(context.Background(), []string{"check", "--help"}, &out, &errout) != 0 || !strings.Contains(errout.String(), "resources") {
		t.Fatal("check help is inaccurate", errout.String())
	}
}
