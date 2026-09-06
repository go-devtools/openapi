package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Forward CLI bases, dialects, and manifest resources while projecting actual Go types.
// CLI 将基准、方言及清单资源送入共享导出器，输出仍来自真实 Go 类型。
func TestSchemaCLIResources(t *testing.T) {
	dir := t.TempDir()
	resource := filepath.Join(dir, "extra.json")
	manifest := filepath.Join(dir, "resources.json")
	output := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(resource, []byte(`{"$id":"extra","type":"string"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`[{"uri":"https://example.test/api/extra","file":"extra.json"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	code := run(context.Background(), []string{"schema", "--dir", "../../testdata/types", "--type", "Request", "--projection", "request", "--base-uri", "https://example.test/api/export.json", "--resources", manifest, "--dialect", "https://json-schema.org/draft/2020-12/schema", "--output", output}, &out, &errs)
	if code != 0 {
		t.Fatalf("exit=%d %s", code, errs.String())
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value["$id"] != "https://example.test/api/export.json" || !bytes.Contains(raw, []byte(`"$id":"https://example.test/api/extra"`)) || !bytes.Contains(raw, []byte(`"minLength":3`)) {
		t.Fatalf("flags or source constraints lost: %s", raw)
	}
}

// Exit successfully for help and reject positional arguments and invalid budgets.
// 帮助正常退出，位置参数和非法预算不能被静默忽略。
func TestSchemaCLIArguments(t *testing.T) {
	for _, sample := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"schema", "--help"}, true},
		{[]string{"schema", "--dir", "../../testdata/types", "--type", "Role", "unexpected"}, false},
		{[]string{"schema", "--dir", "../../testdata/types", "--type", "Role", "--max-normalized-bytes", "-1"}, false},
	} {
		var out, errs bytes.Buffer
		code := run(context.Background(), sample.args, &out, &errs)
		if (code == 0) != sample.ok {
			t.Errorf("args=%v exit=%d stderr=%s", sample.args, code, errs.String())
		}
	}
}

// Preserve existing output and leave no temporary files on resource or cancellation failure.
// 资源读取或取消失败时保留已有输出，不留下临时文件。
func TestSchemaCLIFailurePreservesOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "schema.json")
	before := []byte("existing output")
	if err := os.WriteFile(output, before, 0600); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{context.Background(), cancelled} {
		var out, errs bytes.Buffer
		code := run(ctx, []string{"schema", "--dir", "../../testdata/types", "--type", "Role", "--resources", filepath.Join(dir, "missing.json"), "--output", output}, &out, &errs)
		if code == 0 {
			t.Fatal("invalid export succeeded")
		}
		after, err := os.ReadFile(output)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("existing output changed")
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 1 {
			t.Fatal("temporary output leaked")
		}
	}
}
