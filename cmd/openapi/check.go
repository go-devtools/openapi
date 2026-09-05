package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/openapi-golang/openapi"
)

// 清单显式绑定检索 URI 与本地文件，绝不从规范中的 URI 自动取文件。
// Explicitly map retrieval URIs to local files; never load files from URIs found in the specification.
type resourceFile struct {
	URI  string `json:"uri"`
	File string `json:"file"`
	Kind string `json:"kind,omitempty"`
}

// 在普通文件的分块读取间响应取消，检查命令不执行文件内容。
// Honor cancellation between regular-file reads without executing file contents.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// 每次底层读取前检查取消状态。
// Check cancellation before each underlying read.
func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// 读取用户指定的普通文件，并在分配超过预算前停止。
// Read a user-selected regular file and stop before allocating beyond the budget.
func readBoundedFile(ctx context.Context, path string, limit int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, fmt.Errorf("openapi.cli.budget: 文件读取预算不足")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("openapi.cli.file: 只接受普通文件：%s", path)
	}
	if info.Size() > int64(limit) {
		return nil, fmt.Errorf("openapi.cli.budget: 文件超过剩余字节预算：%s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := contextReader{ctx: ctx, reader: file}
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)))
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	n, err := reader.Read(extra[:])
	if n > 0 {
		return nil, fmt.Errorf("openapi.cli.budget: 文件在读取期间超过预算：%s", path)
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	return data, nil
}

// 在共享字节预算内读取主文档与清单条目，清单本身单独限制为一 MiB。
// Share the byte budget across the root and manifest entries; limit the manifest itself to one MiB.
func readCheckInputs(ctx context.Context, specFile, manifestFile string, options openapi.CheckOptions) ([]byte, openapi.CheckOptions, error) {
	if options.MaxBytes < 1 || options.MaxResources < 1 || options.MaxReferences < 1 || options.MaxIndexBytes < 0 {
		return nil, options, fmt.Errorf("openapi.cli.budget: 所有预算必须大于零")
	}
	raw, err := readBoundedFile(ctx, specFile, options.MaxBytes)
	if err != nil {
		return nil, options, err
	}
	return readResourceManifest(ctx, raw, manifestFile, options)
}

// 对显式清单应用共享读取预算，供文档检查及 Schema 导出复用。
// Apply shared read budgets to explicit manifests for document checks and schema exports.
func readResourceManifest(ctx context.Context, raw []byte, manifestFile string, options openapi.CheckOptions) ([]byte, openapi.CheckOptions, error) {
	if options.MaxBytes < 1 || options.MaxResources < 1 || options.MaxReferences < 1 || options.MaxIndexBytes < 0 {
		return nil, options, fmt.Errorf("openapi.cli.budget: 所有预算必须大于零")
	}
	if manifestFile == "" {
		return raw, options, nil
	}
	manifest, err := readBoundedFile(ctx, manifestFile, 1<<20)
	if err != nil {
		return nil, options, err
	}
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	var entries []resourceFile
	if err = decoder.Decode(&entries); err != nil {
		return nil, options, fmt.Errorf("openapi.cli.resources: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return nil, options, fmt.Errorf("openapi.cli.resources: 清单必须恰好包含一个 JSON 数组")
	}
	if len(entries) >= options.MaxResources {
		return nil, options, fmt.Errorf("openapi.cli.budget: 清单加主文档超过资源数量预算")
	}
	remaining := options.MaxBytes - len(raw)
	options.Resources = map[string][]byte{}
	options.ExampleResources = map[string][]byte{}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.URI == "" || entry.File == "" {
			return nil, options, fmt.Errorf("openapi.cli.resources: 条目必须指定 uri 与 file")
		}
		if entry.Kind != "" && entry.Kind != "document" && entry.Kind != "example" {
			return nil, options, fmt.Errorf("openapi.cli.resources: kind 只能是 document 或 example")
		}
		if seen[entry.URI] {
			return nil, options, fmt.Errorf("openapi.cli.resources: 检索 URI 重复：%s", entry.URI)
		}
		seen[entry.URI] = true
		path := entry.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(manifestFile), path)
		}
		data, err := readBoundedFile(ctx, path, remaining)
		if err != nil {
			return nil, options, err
		}
		remaining -= len(data)
		if entry.Kind == "example" {
			options.ExampleResources[entry.URI] = data
		} else {
			options.Resources[entry.URI] = data
		}
	}
	return raw, options, nil
}
