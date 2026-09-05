package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 验证 CLI 从真实源码生成独立 Schema，且检查命令使用真实输入。
func TestSchemaAndCheckCommands(t *testing.T) {
	output := filepath.Join(t.TempDir(), "schema.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"schema", "--dir", "../../testdata/types", "--type", "Request", "--projection", "request", "--output", output}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("生成失败：%d %s", code, stderr.String())
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "#/$defs/") {
		t.Fatal("没有独立 Schema 引用")
	}
	specFile := filepath.Join(t.TempDir(), "openapi.json")
	if err = os.WriteFile(specFile, []byte(`{"openapi":"3.1.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if run(context.Background(), []string{"check", "--spec", specFile}, &stdout, &stderr) == 0 {
		t.Fatal("错误接受不兼容规范")
	}
}
