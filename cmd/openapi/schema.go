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

// Export schemas from actual source, using only explicitly listed external resources.
// 从真实源码导出 Schema，所有外部内容仅来自显式资源清单。
func runSchema(ctx context.Context, args []string, stdout, stderr io.Writer, fail func(error) int) int {
	flags := flag.NewFlagSet("schema", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", ".", "Project directory to analyze")
	name := flags.String("type", "", "Actual Go type reference")
	direction := flags.String("projection", "response", "request or response")
	output := flags.String("output", "", "Output file; write to standard output when omitted")
	base := flags.String("base-uri", "", "Absolute retrieval URI for the root Schema")
	dialect := flags.String("dialect", "", "Dialect URI for a root without a declaration; defaults to JSON Schema 2020-12")
	manifest := flags.String("resources", "", "Explicit offline JSON Schema resource manifest")
	maxBytes := flags.Int("max-bytes", 8<<20, "Maximum total input bytes for the root and preloaded content")
	maxResources := flags.Int("max-resources", 64, "Maximum count of main document, preloaded resources, and embedded identities")
	maxReferences := flags.Int("max-references", 10000, "Maximum reference count")
	maxIndex := flags.Int("max-index-bytes", 16<<20, "Text budget for indexing and reference processing")
	maxOutput := flags.Int("max-normalized-bytes", 16<<20, "Maximum output bytes for standalone Schema")
	maxTypes := flags.Int("max-types", 4096, "Maximum type projection count")
	maxPackages := flags.Int("max-packages", 2048, "Maximum statically loaded package count")
	timeout := flags.Duration("timeout", time.Minute, "Total duration budget for source loading and export")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return fail(fmt.Errorf("schema does not accept positional arguments"))
	}
	if *name == "" {
		return fail(fmt.Errorf("--type is required"))
	}
	if *timeout <= 0 || *maxTypes < 1 || *maxPackages < 1 || *maxOutput < 1 {
		return fail(fmt.Errorf("resource budgets must be positive"))
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	_, resources, err := readResourceManifest(ctx, nil, *manifest, openapi.CheckOptions{BaseURI: *base, MaxBytes: *maxBytes, MaxResources: *maxResources, MaxReferences: *maxReferences, MaxIndexBytes: *maxIndex})
	if err != nil {
		return fail(err)
	}
	if len(resources.ExampleResources) > 0 {
		return fail(fmt.Errorf("openapi.schema.resources: standalone Schema manifest does not accept example entries"))
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
		return fail(fmt.Errorf("openapi.schema.budget: output and trailing newline exceed --max-normalized-bytes"))
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

// Complete a same-directory temporary file before replacing the target; preserve it on failure or cancellation.
// 在同目录写完临时文件再替换目标，失败或取消时保留原文件。
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
