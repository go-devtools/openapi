package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/compiler"
)

// 从真实源码导出 Schema，所有外部内容仅来自显式资源清单。
// Export schemas from actual source, using only explicitly listed external resources.
func runSchema(ctx context.Context, args []string, stdout, stderr io.Writer, fail func(error) int) int {
	flags := flag.NewFlagSet("schema", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "待分析项目目录")
	name := flags.String("type", "", "真实 Go 类型引用")
	direction := flags.String("projection", "response", "request 或 response")
	output := flags.String("output", "", "输出文件，省略时写到标准输出")
	base := flags.String("base-uri", "", "根 Schema 的绝对检索 URI")
	dialect := flags.String("dialect", "", "根未声明方言时采用的 URI，默认 JSON Schema 2020-12")
	manifest := flags.String("resources", "", "显式离线 JSON Schema 资源清单")
	maxBytes := flags.Int("max-bytes", 8<<20, "根及预载内容的总输入字节上限")
	maxResources := flags.Int("max-resources", 64, "主文档、预载资源和嵌入身份数量上限")
	maxReferences := flags.Int("max-references", 10000, "引用数量上限")
	maxIndex := flags.Int("max-index-bytes", 16<<20, "索引与引用处理的文本预算")
	maxOutput := flags.Int("max-normalized-bytes", 16<<20, "独立 Schema 输出字节上限")
	maxTypes := flags.Int("max-types", 4096, "类型投影数量上限")
	maxPackages := flags.Int("max-packages", 2048, "静态加载包数量上限")
	timeout := flags.Duration("timeout", time.Minute, "源码加载及导出的总耗时预算")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return fail(fmt.Errorf("schema 不接受未命名参数"))
	}
	if *name == "" {
		return fail(fmt.Errorf("缺少 --type"))
	}
	if *timeout <= 0 || *maxTypes < 1 || *maxPackages < 1 || *maxOutput < 1 {
		return fail(fmt.Errorf("资源预算必须为正"))
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	_, resources, err := readResourceManifest(ctx, nil, *manifest, openapi.CheckOptions{BaseURI: *base, MaxBytes: *maxBytes, MaxResources: *maxResources, MaxReferences: *maxReferences, MaxIndexBytes: *maxIndex})
	if err != nil {
		return fail(err)
	}
	if len(resources.ExampleResources) > 0 {
		return fail(fmt.Errorf("openapi.schema.resources: 独立 Schema 清单不接受 example 条目"))
	}
	project, err := compiler.Load(ctx, compiler.LoadOptions{Dir: *dir, MaxPackages: *maxPackages})
	if err != nil {
		return fail(err)
	}
	typ, err := project.Type(*name)
	if err != nil {
		return fail(err)
	}
	projected, err := project.Schema(compiler.ProjectionRequest{Type: typ, Direction: compiler.Direction(*direction), MediaType: "application/json", MaxTypes: *maxTypes})
	if err != nil {
		return fail(err)
	}
	raw, err := projected.StandaloneWithOptions(compiler.StandaloneOptions{BaseURI: *base, Dialect: *dialect, Resources: resources.Resources, MaxBytes: *maxBytes, MaxResources: *maxResources, MaxReferences: *maxReferences, MaxIndexBytes: *maxIndex, MaxNormalizedBytes: *maxOutput})
	if err != nil {
		return fail(err)
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	if len(raw) >= *maxOutput {
		return fail(fmt.Errorf("openapi.schema.budget: 输出与末尾换行超过 --max-normalized-bytes"))
	}
	raw = append(raw, '\n')
	if *output == "" {
		_, err = stdout.Write(raw)
	} else {
		err = writeSchemaFile(ctx, *output, raw)
	}
	if err != nil {
		return fail(err)
	}
	return 0
}

// 在同目录写完临时文件再替换目标，失败或取消时保留原文件。
// Complete a same-directory temporary file before replacing the target; preserve it on failure or cancellation.
func writeSchemaFile(ctx context.Context, path string, raw []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".openapi-schema-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = file.Chmod(0644); err != nil {
		return err
	}
	if _, err = file.Write(raw); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
