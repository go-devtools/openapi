package compiler_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
)

// Generic constraints are not runtime interfaces; preserve unresolved zero values and concrete argument nilness.
func TestTypeParameterValues(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{
		"go.mod": "module example.test/type-parameter-values\n\ngo 1.27.1\n",
		"app.go": `package sample
type Item struct{ Name string }
func HZero[T any]() any { var value T; return value }
func HScalarZero[T ~int]() any { var value T; return value }
func HBoxedZero[T any]() any { var value T; var boxed any = value; return boxed }
func HNestedZero[T any]() any { var value struct{ Data T }; return value.Data }
func HConverted[T ~int]() any { return T(1) }
func HConditional[T any]() any { var value T; if any(value) == nil { return "nil" }; return 42 }
func HPointer[T any]() any { var value *T; return value }
func HSlice[T any]() any { var value []T; return value }
func HMap[T any]() any { var value map[string]T; return value }
func identity[T any](value T) T { return value }
func pointerNil[T ~*Item](value T) bool { return value == nil }
func HConcrete() any { return identity(7) }
func HNilArgument() any { var value *Item; if pointerNil(value) { return "nil" }; return 42 }
func HNonNilArgument() any { value := &Item{}; if pointerNil(value) { return "nil" }; return 42 }
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := core.Compile(context.Background(), core.Options{
		Load: core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}},
		Frontends: []core.Frontend{{Name: "neutral-generic-values-v1", Match: func(function core.Function) bool {
			return strings.HasPrefix(function.Object.Name(), "H")
		}, Return: func(call core.ReturnContext) ([]core.Effect, error) {
			return []core.Effect{{Kind: core.ResponseBody, Status: "200", MediaType: "application/json", Payload: call.Values[0], Source: call.Source}}, nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, entry := range result.Bundle.Index() {
		name := strings.TrimPrefix(entry.Symbol, "example.test/type-parameter-values.")
		t.Run(name, func(t *testing.T) {
			document, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Generic values", Version: "1"})
			if name == "HConverted" {
				if err == nil {
					t.Fatal("conversion to an uninstantiated type parameter erased its wire identity")
				}
				return
			}
			if strings.HasSuffix(name, "Zero") {
				if err == nil || !strings.Contains(err.Error(), "critical payload type is unresolved") {
					t.Fatalf("unbound type parameter became a trusted payload: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			validator, err := contracttest.Compile(document.JSON(), "/paths/~1value/get/responses/200/content/application~1json/schema", contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{"null", `"nil"`, "42"} {
				want := value == "null"
				switch name {
				case "HConditional":
					want = value != "null"
				case "HConcrete", "HNonNilArgument":
					want = value == "42"
				case "HNilArgument":
					want = value == `"nil"`
				}
				if accepted := validator.JSON([]byte(value)) == nil; accepted != want {
					t.Fatalf("value %s accepted=%v, want %v; %s", value, accepted, want, document.JSON())
				}
			}
		})
		seen++
	}
	if seen != 12 {
		t.Fatalf("expected twelve actual source candidates, got %d", seen)
	}
}
