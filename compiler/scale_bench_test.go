package compiler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/compiler"
)

// Retain returned snapshots so allocation measurements include their public defensive copies.
var scaleJSON []byte

// Compile distinct neutral handlers from actual tag-free Go source.
func scaleProject(b *testing.B, count int) (compiler.Options, []openapi.Route) {
	b.Helper()
	dir := b.TempDir()
	var source strings.Builder
	source.WriteString("package scale\n\n// Creation input.\ntype Request struct {\n// Display name.\n// @openapi required minLength=3\nName string\n}\n")
	routes := make([]openapi.Route, count)
	for i := range routes {
		name := fmt.Sprintf("Handle%04d", i)
		fmt.Fprintf(&source, "\n// Create an item.\nfunc %s(input Request) Request { return input }\n", name)
		routes[i] = openapi.Route{Method: "POST", Path: fmt.Sprintf("/items/%d", i), OperationKey: openapi.OperationKey("example.test/core-scale." + name)}
	}
	for name, content := range map[string]string{"go.mod": "module example.test/core-scale\n\ngo 1.27.1\n", "api.go": source.String()} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			b.Fatal(err)
		}
	}
	frontend := compiler.Frontend{Name: "scale-return-v1", Match: func(f compiler.Function) bool {
		return strings.HasPrefix(f.Object.Name(), "Handle")
	}, Entry: func(f compiler.Function) []compiler.Effect {
		return []compiler.Effect{{Kind: compiler.RequestBody, MediaType: "application/json", Required: true, Payload: compiler.Value{Type: f.Signature.Params().At(0).Type()}, Source: f.Source}}
	}, Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "200", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
	}}
	return compiler.Options{Load: compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{frontend}}, routes
}

// Measure full source generation separately from startup Build and cached document reads.
func BenchmarkScale(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("routes=%d", count), func(b *testing.B) {
			options, routes := scaleProject(b, count)
			result, err := compiler.Compile(context.Background(), options)
			if err != nil {
				b.Fatal(err)
			}
			if len(result.Bundle.Index()) != count {
				b.Fatal("source generation lost distinct handler templates")
			}
			config := openapi.Config{Title: "Neutral scale", Version: "1"}
			document, err := openapi.Build(result.Bundle, routes, config)
			if err != nil {
				b.Fatal(err)
			}
			var decoded struct{ Paths map[string]json.RawMessage }
			if err := json.Unmarshal(document.JSON(), &decoded); err != nil || len(decoded.Paths) != count {
				b.Fatalf("expected %d generated paths: %v", count, err)
			}
			size := len(document.JSON())
			b.Run("Generate", func(b *testing.B) {
				output := compiler.WriteOptions{Dir: filepath.Join(options.Load.Dir, "internal", "apidoc")}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					compiled, err := compiler.Compile(context.Background(), options)
					if err != nil {
						b.Fatal(err)
					}
					if err := compiled.Write(output); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("Build", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := openapi.Build(result.Bundle, routes, config); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("Read", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					scaleJSON = document.JSON()
				}
			})
		})
	}
}
