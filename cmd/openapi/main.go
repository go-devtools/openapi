// Provide framework-neutral Schema, specification, and version commands.
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

	"github.com/go-devtools/openapi"
)

// Cancel on interruption without running project scripts.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// Dispatch core commands without embedding framework frontends.
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
		_ = json.NewEncoder(stdout).Encode(map[string]any{"module": "github.com/go-devtools/openapi", "version": version, "revision": revision, "go": runtime.Version(), "bundle": openapi.BundleFormatVersion, "openapi": "3.2.0"})
		return 0
	case "check", "check-spec":
		flags := flag.NewFlagSet("check", flag.ContinueOnError)
		flags.SetOutput(stderr)
		file := flags.String("spec", "", "OpenAPI JSON file to validate")
		base := flags.String("base-uri", "", "Absolute retrieval URI for the main document; used only for resolution, without downloading content")
		manifest := flags.String("resources", "", "Offline resource manifest JSON; entries contain uri, file, and optional kind")
		maxBytes := flags.Int("max-bytes", 8<<20, "Maximum total bytes for the main document and preloaded content")
		maxResources := flags.Int("max-resources", 64, "Maximum resource count including the main document and embedded $id values")
		maxReferences := flags.Int("max-references", 10000, "Maximum specification reference count")
		maxIndexBytes := flags.Int("max-index-bytes", 16<<20, "Maximum cumulative bytes for indexing, URI resolution, and diagnostic text")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if flags.NArg() != 0 {
			return fail(fmt.Errorf("check does not accept positional arguments"))
		}
		if *file == "" {
			return fail(fmt.Errorf("--spec is required"))
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
		return runSchema(ctx, args[1:], stdout, stderr, fail)
	default:
		return fail(fmt.Errorf("unknown command %s", args[0]))
	}
}
