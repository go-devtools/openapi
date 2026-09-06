package consumer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
)

// 以真正源码验证函数值、捕获单元和接收者，不执行待分析业务函数。
// Verify function values, captured cells, and receivers from real source without executing analyzed business functions.
func TestFunctionValues(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
// 中立输出通道。 A neutral output channel.
type Channel struct{}
// 写入状态。 Write a status.
func (*Channel) Write(int){}
// 分离每次工厂调用的捕获值。 Separate captures from each factory invocation.
func factory(code int) func() int { return func() int { return code } }
// 别名共享同一可变捕获单元。 Aliases share one mutable capture cell.
func counter(code int) func() int { return func() int {code=code+1;return code} }
// 通过函数参数调用。 Invoke through a function parameter.
func apply(f func() int) int {return f()}
// 保存值接收者状态。 Hold value-receiver state.
type Receiver struct{Code int}
// 读取值接收者。 Read a value receiver.
func (r Receiver) CodeValue() int {return r.Code}
// 读取指针接收者。 Read a pointer receiver.
func (r *Receiver) CodePointer() int {return r.Code}
// 在调用时读取捕获变量。 Read the captured variable at invocation.
func HLocal(c *Channel){code:=201;f:=func(){c.Write(code)};code=202;f()}
// 不同工厂实例互不覆盖。 Different factory instances do not overwrite one another.
func HFactory(c *Channel){a,b:=factory(201),factory(202);c.Write(a()+b()-200)}
// 函数别名共享修改。 Function aliases share mutations.
func HAlias(c *Channel){f:=counter(201);g:=f;g();c.Write(f())}
// 外层 helper 保留函数值。 Outer helpers preserve function values.
func HHigher(c *Channel){c.Write(apply(factory(207)))}
// 分支捕获值隔离。 Captured values stay isolated across branches.
func HBranch(c *Channel,flag bool){code:=200;f:=func(){c.Write(code)};if flag{code=201}else{code=202};f()}
// 值方法保存创建时的接收者。 Value methods retain the receiver from their creation.
func HValueMethod(c *Channel){r:=Receiver{201};f:=r.CodeValue;r=Receiver{202};c.Write(f())}
// 指针方法读取实际共享单元。 Pointer methods read the actual shared cell.
func HPointerMethod(c *Channel){r:=Receiver{201};f:=r.CodePointer;r=Receiver{202};c.Write(f())}
// 方法表达式取首个实参为接收者。 Method expressions use their first argument as receiver.
func HMethodExpression(c *Channel){f:=Receiver.CodeValue;c.Write(f(Receiver{203}))}
// 前端规则也识别绑定方法别名。 Frontend rules also recognize bound method aliases.
func HFrontendAlias(c *Channel){f:=c.Write;f(206)}
// 自增修改真实捕获单元。 Increment the actual captured cell.
func HIncrement(c *Channel){code:=201;f:=func(){code++};f();c.Write(code)}
// 字段写入会被指针方法观察到。 Pointer methods observe field writes.
func HFieldMutation(c *Channel){r:=Receiver{201};f:=r.CodePointer;r.Code=204;c.Write(f())}
// 有符号窄整数按实际 Go 位宽回绕。 Narrow signed integers wrap at their actual Go width.
func HOverflow(c *Channel){var n int8=127;n++;if n<0{c.Write(200)}else{c.Write(500)}}
// 未调用的闭包没有响应效果。 Uncalled closures have no response effects.
func HUnused(c *Channel){f:=func(){c.Write(500)};_=f;c.Write(200)}
// 未知函数不能默认为无副作用。 Unknown functions cannot default to effect-free calls.
func HUnknown(c *Channel,f func()){f();c.Write(200)}
// nil 函数不能生成正常契约。 Nil functions cannot produce a normal contract.
func HNil(c *Channel){var f func();f();c.Write(200)}
// 递归闭包仍受深度预算约束。 Recursive closures remain subject to the depth budget.
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
