package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// 输出预算包含 CLI 的末尾换行，不能多写一个未计费字节。
// Include the CLI's trailing newline in the output budget instead of writing an uncharged byte.
func TestSchemaCLIOutputBudgetIncludesNewline(t *testing.T) {
	args := []string{"schema", "--dir", "../../testdata/types", "--type", "Role"}
	var out, errs bytes.Buffer
	if code := run(context.Background(), args, &out, &errs); code != 0 {
		t.Fatal(errs.String())
	}
	size := out.Len()
	for _, limit := range []int{size, size - 1} {
		out.Reset()
		errs.Reset()
		code := run(context.Background(), append(append([]string{}, args...), "--max-normalized-bytes", strconv.Itoa(limit)), &out, &errs)
		if (code == 0) != (limit == size) {
			t.Fatalf("limit=%d size=%d exit=%d stderr=%s", limit, size, code, errs.String())
		}
		if code != 0 && out.Len() != 0 {
			t.Fatal("failed export wrote partial output")
		}
	}
}

// 原子替换失败时删除已写完的临时文件，并保留目标目录的原内容。
// Remove a completed temporary file after failed replacement and preserve the target directory's contents.
func TestSchemaAtomicWriteFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing")
	sentinel := filepath.Join(target, "data")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeSchemaFile(context.Background(), target, []byte("new")); err == nil {
		t.Fatal("replaced a directory with schema bytes")
	}
	raw, err := os.ReadFile(sentinel)
	if err != nil || string(raw) != "keep" {
		t.Fatal("target changed")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("temporary file leaked")
	}
}
