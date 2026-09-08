package consumer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
	core "github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/spec"
)

// Verify function values, captured cells, and receivers from real source without executing analyzed business functions.
func TestFunctionValues(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
// A neutral output channel.
type Channel struct{}
// Write a status.
func (*Channel) Write(int){}
// Separate captures from each factory invocation.
func factory(code int) func() int { return func() int { return code } }
// Aliases share one mutable capture cell.
func counter(code int) func() int { return func() int {code=code+1;return code} }
// Invoke through a function parameter.
func apply(f func() int) int {return f()}
// Hold value-receiver state.
type Receiver struct{Code int}
// Read a value receiver.
func (r Receiver) CodeValue() int {return r.Code}
// Read a pointer receiver.
func (r *Receiver) CodePointer() int {return r.Code}
// Read the captured variable at invocation.
func HLocal(c *Channel){code:=201;f:=func(){c.Write(code)};code=202;f()}
// Different factory instances do not overwrite one another.
func HFactory(c *Channel){a,b:=factory(201),factory(202);c.Write(a()+b()-200)}
// Function aliases share mutations.
func HAlias(c *Channel){f:=counter(201);g:=f;g();c.Write(f())}
// Outer helpers preserve function values.
func HHigher(c *Channel){c.Write(apply(factory(207)))}
// Captured values stay isolated across branches.
func HBranch(c *Channel,flag bool){code:=200;f:=func(){c.Write(code)};if flag{code=201}else{code=202};f()}
// Value methods retain the receiver from their creation.
func HValueMethod(c *Channel){r:=Receiver{201};f:=r.CodeValue;r=Receiver{202};c.Write(f())}
// Pointer methods read the actual shared cell.
func HPointerMethod(c *Channel){r:=Receiver{201};f:=r.CodePointer;r=Receiver{202};c.Write(f())}
// Method expressions use their first argument as receiver.
func HMethodExpression(c *Channel){f:=Receiver.CodeValue;c.Write(f(Receiver{203}))}
// Frontend rules also recognize bound method aliases.
func HFrontendAlias(c *Channel){f:=c.Write;f(206)}
// Increment the actual captured cell.
func HIncrement(c *Channel){code:=201;f:=func(){code++};f();c.Write(code)}
// Pointer methods observe field writes.
func HFieldMutation(c *Channel){r:=Receiver{201};f:=r.CodePointer;r.Code=204;c.Write(f())}
// Narrow signed integers wrap at their actual Go width.
func HOverflow(c *Channel){var n int8=127;n++;if n<0{c.Write(200)}else{c.Write(500)}}
// Uncalled closures have no response effects.
func HUnused(c *Channel){f:=func(){c.Write(500)};_=f;c.Write(200)}
// Unknown functions cannot default to effect-free calls.
func HUnknown(c *Channel,f func()){f();c.Write(200)}
// Nil functions cannot produce a normal contract.
func HNil(c *Channel){var f func();f();c.Write(200)}
// Recursive closures remain subject to the depth budget.
func HRecursive(c *Channel){var f func();f=func(){f()};f();c.Write(200)}
`
	for name, text := range map[string]string{"go.mod": "module example.test/function-values\n\ngo 1.27.1\n", "app.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	front := core.Frontend{Name: "neutral-function-values-v1", Match: func(f core.Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, Call: func(c core.CallContext) ([]core.Effect, error) {
		if c.Object == nil || c.Object.Name() != "Write" {
			return nil, nil
		}
		status := ""
		if len(c.Arguments) == 1 && c.Arguments[0].Constant != nil {
			status = c.Arguments[0].Constant.ExactString()
		}
		return []core.Effect{{Kind: core.ResponseBody, Status: status, MediaType: "text/plain", WireSchema: spec.Typed("string"), Source: c.Source}}, nil
	}}
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{front}, MaxDepth: 8})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string][]string{"HLocal": {"202"}, "HFactory": {"203"}, "HAlias": {"203"}, "HHigher": {"207"}, "HBranch": {"201", "202"}, "HValueMethod": {"201"}, "HPointerMethod": {"202"}, "HMethodExpression": {"203"}, "HFrontendAlias": {"206"}, "HUnused": {"200"}, "HIncrement": {"202"}, "HFieldMutation": {"204"}, "HOverflow": {"200"}, "HUnknown": nil, "HNil": nil, "HRecursive": nil}
	if len(result.Bundle.Index()) != len(expected) {
		t.Fatal("missing candidates")
	}
	for _, entry := range result.Bundle.Index() {
		name := strings.TrimPrefix(entry.Symbol, "example.test/function-values.")
		t.Run(name, func(t *testing.T) {
			_, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Function values", Version: "1"})
			if expected[name] == nil {
				if err == nil {
					t.Fatal("unknown, nil, or recursive callback was trusted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var actual []string
			for status := range entry.Operation.Responses {
				actual = append(actual, status)
			}
			sort.Strings(actual)
			if !reflect.DeepEqual(actual, expected[name]) {
				t.Fatalf("statuses %v, want %v", actual, expected[name])
			}
		})
	}
}
