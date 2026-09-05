// 提供框架无关的 Schema、规范检查与版本命令。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/compiler"
)

// 响应中断取消，不运行用户项目脚本。
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// 分派核心命令；命令编排不包含任何框架前端。
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		_ = json.NewEncoder(stderr).Encode(map[string]any{"code": "openapi.cli.failed", "severity": "error", "message": err.Error()})
		return 1
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(stdout, "openapi schema --dir . --type Type --projection request|response --output schema.json\nopenapi check --spec openapi.json [--base-uri URI] [--resources resources.json]\nopenapi version")
		return 0
	}
	switch args[0] {
	case "version":
		version := "unversioned"
		revision := ""
		if info, ok := debug.ReadBuildInfo(); ok {
			version = info.Main.Version
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					revision = setting.Value
				}
			}
		}
		_ = json.NewEncoder(stdout).Encode(map[string]any{"module": "github.com/openapi-golang/openapi", "version": version, "revision": revision, "go": runtime.Version(), "bundle": openapi.BundleFormatVersion, "openapi": "3.2.0"})
		return 0
	case "check", "check-spec":
		flags := flag.NewFlagSet("check", flag.ContinueOnError)
		flags.SetOutput(stderr)
		file := flags.String("spec", "", "待校验的 OpenAPI JSON 文件")
		base := flags.String("base-uri", "", "主文档的绝对检索 URI；仅用于解析，不下载内容")
		manifest := flags.String("resources", "", "离线资源清单 JSON；条目包含 uri、file 和可选 kind")
		maxBytes := flags.Int("max-bytes", 8<<20, "主文档与预载内容的总字节上限")
		maxResources := flags.Int("max-resources", 64, "包括主文档及内嵌 $id 的资源数量上限")
		maxReferences := flags.Int("max-references", 10000, "规范引用次数上限")
		maxIndexBytes := flags.Int("max-index-bytes", 16<<20, "索引、URI 解析和诊断文本的累计字节上限")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if flags.NArg() != 0 {
			return fail(fmt.Errorf("check 不接受未命名参数"))
		}
		if *file == "" {
			return fail(fmt.Errorf("缺少 --spec"))
		}
		options := openapi.CheckOptions{BaseURI: *base, MaxBytes: *maxBytes, MaxResources: *maxResources, MaxReferences: *maxReferences, MaxIndexBytes: *maxIndexBytes}
		raw, options, err := readCheckInputs(ctx, *file, *manifest, options)
		if err != nil {
			return fail(err)
		}
		if err = ctx.Err(); err != nil {
			return fail(err)
		}
		report := openapi.CheckWithOptions(raw, options)
		if err = ctx.Err(); err != nil {
			return fail(err)
		}
		_ = json.NewEncoder(stdout).Encode(report)
		if report.HasErrors() {
			return 1
		}
		return 0
	case "schema":
		flags := flag.NewFlagSet("schema", flag.ContinueOnError)
		flags.SetOutput(stderr)
		dir := flags.String("dir", ".", "待分析项目目录")
		name := flags.String("type", "", "真实 Go 类型引用")
		direction := flags.String("projection", "response", "request 或 response")
		output := flags.String("output", "", "输出文件，省略时写到标准输出")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		if *name == "" {
			return fail(fmt.Errorf("缺少 --type"))
		}
		project, err := compiler.Load(ctx, compiler.LoadOptions{Dir: *dir})
		if err != nil {
			return fail(err)
		}
		typ, err := project.Type(*name)
		if err != nil {
			return fail(err)
		}
		schema, err := project.Schema(compiler.ProjectionRequest{Type: typ, Direction: compiler.Direction(*direction), MediaType: "application/json"})
		if err != nil {
			return fail(err)
		}
		raw, err := schema.Standalone()
		if err != nil {
			return fail(err)
		}
		raw = append(raw, '\n')
		if *output == "" {
			_, err = stdout.Write(raw)
		} else {
			err = os.WriteFile(*output, raw, 0644)
		}
		if err != nil {
			return fail(err)
		}
		return 0
	default:
		return fail(fmt.Errorf("未知命令 %s", args[0]))
	}
}
