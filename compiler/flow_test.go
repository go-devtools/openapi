package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
)

// Test nested statements and status commits using a neutral effect carrier.
// 用框架中立替身验证嵌套语句、状态提交与多返回值传播。
func TestFlowWriteOrdering(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
// Carry protocol effects.
type Context struct{}
// Provide a static payload sample.
type Payload struct { Name string }
func (*Context) Read() error { return nil }
func (*Context) Key() string { return "" }
func (*Context) Status(int) {}
func (*Context) Commit(int) {}
func (*Context) Write(int, any) {}
func HInline(c *Context) { if c.Read()!=nil {c.Write(400,Payload{});return};c.Write(200,Payload{}) }
func HPending(c *Context) { c.Status(202);if c.Key()=="created" {c.Status(201)};c.Write(-1,Payload{}) }
func HSwitch(c *Context) {c.Status(202);switch c.Key(){case "break":break;default:c.Status(201)};c.Write(-1,Payload{})}
func HCommit(c *Context) {c.Commit(403);c.Write(200,Payload{})}
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/flow\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	front := Frontend{Name: "test-neutral-flow", Match: func(f Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, Call: func(c CallContext) ([]Effect, error) {
		if c.Object == nil {
			return nil, nil
		}
		status := func() string {
			if len(c.Arguments) > 0 && c.Arguments[0].Constant != nil {
				return c.Arguments[0].Constant.ExactString()
			}
			return ""
		}
		switch c.Object.Name() {
		case "Read":
			return []Effect{{Kind: RequestBody, MediaType: "application/json", Payload: Value{Type: c.Function.Package.Types.Scope().Lookup("Payload").Type()}, Source: c.Source}}, nil
		case "Key":
			return []Effect{{Kind: Handled}}, nil
		case "Status":
			return []Effect{{Kind: ResponseStatus, Status: status(), Source: c.Source}}, nil
		case "Commit":
			return []Effect{{Kind: ResponseCommit, Status: status(), Source: c.Source}}, nil
		case "Write":
			return []Effect{{Kind: ResponseBody, Status: status(), MediaType: "application/json", Payload: c.Arguments[1], Source: c.Source}}, nil
		}
		return nil, nil
	}}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{front}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Bundle.Index() {
		t.Run(entry.Symbol, func(t *testing.T) {
			doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/sample", OperationKey: entry.Key}}, openapi.Config{Title: "Test", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			switch entry.Symbol {
			case "example.com/flow.HInline":
				if !strings.Contains(string(doc.JSON()), `"requestBody"`) {
					t.Fatal("request read in the if condition was omitted")
				}
			case "example.com/flow.HPending", "example.com/flow.HSwitch":
				if len(entry.Operation.Responses) != 2 || entry.Operation.Responses["201"].Value == nil || entry.Operation.Responses["202"].Value == nil {
					t.Fatalf("incorrect status branch: %s", doc.JSON())
				}
			case "example.com/flow.HCommit":
				if len(entry.Operation.Responses) != 1 || entry.Operation.Responses["403"].Value == nil {
					t.Fatal("subsequent call overwrote committed status")
				}
			}
		})
	}
}
