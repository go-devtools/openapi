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

// Explicitly map retrieval URIs to local files; never load files from URIs found in the specification.
// 清单显式绑定检索 URI 与本地文件，绝不从规范中的 URI 自动取文件。
type resourceFile struct {
	URI  string `json:"uri"`
	File string `json:"file"`
	Kind string `json:"kind,omitempty"`
}

// Honor cancellation between regular-file reads without executing file contents.
// 在普通文件的分块读取间响应取消，检查命令不执行文件内容。
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Check cancellation before each underlying read.
// 每次底层读取前检查取消状态。
func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Read a user-selected regular file and stop before allocating beyond the budget.
// 读取用户指定的普通文件，并在分配超过预算前停止。
func readBoundedFile(ctx context.Context, path string, limit int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, fmt.Errorf("openapi.cli.budget: insufficient file read budget")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("openapi.cli.file: only regular files are accepted: %s", path)
	}
	if info.Size() > int64(limit) {
		return nil, fmt.Errorf("openapi.cli.budget: file exceeds the remaining byte budget: %s", path)
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
		return nil, fmt.Errorf("openapi.cli.budget: file exceeded the budget while being read: %s", path)
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	return data, nil
}

// Share the byte budget across the root and manifest entries; limit the manifest itself to one MiB.
// 在共享字节预算内读取主文档与清单条目，清单本身单独限制为一 MiB。
func readCheckInputs(ctx context.Context, specFile, manifestFile string, options openapi.CheckOptions) ([]byte, openapi.CheckOptions, error) {
	if options.MaxBytes < 1 || options.MaxResources < 1 || options.MaxReferences < 1 || options.MaxIndexBytes < 0 {
		return nil, options, fmt.Errorf("openapi.cli.budget: all budgets must be greater than zero")
	}
	raw, err := readBoundedFile(ctx, specFile, options.MaxBytes)
	if err != nil {
		return nil, options, err
	}
	return readResourceManifest(ctx, raw, manifestFile, options)
}

// Apply shared read budgets to explicit manifests for document checks and schema exports.
// 对显式清单应用共享读取预算，供文档检查及 Schema 导出复用。
func readResourceManifest(ctx context.Context, raw []byte, manifestFile string, options openapi.CheckOptions) ([]byte, openapi.CheckOptions, error) {
	if options.MaxBytes < 1 || options.MaxResources < 1 || options.MaxReferences < 1 || options.MaxIndexBytes < 0 {
		return nil, options, fmt.Errorf("openapi.cli.budget: all budgets must be greater than zero")
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
		return nil, options, fmt.Errorf("openapi.cli.resources: manifest must contain exactly one JSON array")
	}
	if len(entries) >= options.MaxResources {
		return nil, options, fmt.Errorf("openapi.cli.budget: manifest and main document exceed the resource count budget")
	}
	remaining := options.MaxBytes - len(raw)
	options.Resources = map[string][]byte{}
	options.ExampleResources = map[string][]byte{}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.URI == "" || entry.File == "" {
			return nil, options, fmt.Errorf("openapi.cli.resources: entries must specify uri and file")
		}
		if entry.Kind != "" && entry.Kind != "document" && entry.Kind != "example" {
			return nil, options, fmt.Errorf("openapi.cli.resources: kind must be document or example")
		}
		if seen[entry.URI] {
			return nil, options, fmt.Errorf("openapi.cli.resources: duplicate retrieval URI: %s", entry.URI)
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
