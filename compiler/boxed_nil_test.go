package compiler_test

import (
	"context"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
	core "github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/contracttest"
)

// Interface boxing preserves both non-nil interface identity and its nil payload for correct branching and serialization.
func TestBoxedNilPayload(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.test/boxed-nil\n\ngo 1.27.1\n",
		"app.go": `package sample
type Item struct{ Name string }
type Envelope struct{ Nested struct{ Data any } }
func HZero() any { var value Envelope; return value.Nested.Data }
func HPointer() any { var p *Item; var boxed any = p; if boxed == nil { return Item{"wrong branch"} }; return boxed }
func HSlice() any { var p []string; var boxed any = p; return boxed }
func HMap() any { var p map[string]int; var boxed any = p; return boxed }
func HNil() any { return nil }
func HConvertedNil() any { return any((*Item)(nil)) }
func HUnknown(p *Item) any { return p }
func HNonNil() any { return any(&Item{Name:"Ada"}) }
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{{Name: "neutral-boxed-nil-v1", Match: func(f core.Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, Return: func(c core.ReturnContext) ([]core.Effect, error) {
		if c.Function.Object.Name() == "HUnknown" {
			if !c.Values[0].Boxed || c.Values[0].DynamicNil || c.Values[0].DynamicNonNil {
				t.Error("unknown pointer nilness became definite after boxing")
			}
		}
		if c.Function.Object.Name() == "HNonNil" {
			if !c.Values[0].Boxed || !c.Values[0].DynamicNonNil {
				t.Error("known concrete pointer identity was lost")
			}
		}
		if c.Function.Object.Name() == "HConvertedNil" {
			if _, ok := c.Values[0].Type.(*types.Pointer); !ok {
				t.Error("interface conversion erased the concrete payload type")
			}
		}
		return []core.Effect{{Kind: core.ResponseBody, Status: "200", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Bundle.Index() {
		if strings.HasSuffix(entry.Symbol, ".HUnknown") || strings.HasSuffix(entry.Symbol, ".HNonNil") {
			continue
		}
		t.Run(entry.Symbol, func(t *testing.T) {
			doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/nil", OperationKey: entry.Key}}, openapi.Config{Title: "Nil", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			v, err := contracttest.Compile(doc.JSON(), "/paths/~1nil/get/responses/200/content/application~1json/schema", contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err = v.JSON([]byte("null")); err != nil {
				t.Fatal(err)
			}
			for _, unexpected := range []string{`{"Name":"wrong branch"}`, `["invented"]`, `{"key":1}`} {
				if v.JSON([]byte(unexpected)) == nil {
					t.Fatalf("boxed nil lost its payload identity: %s", unexpected)
				}
			}
		})
	}
}
