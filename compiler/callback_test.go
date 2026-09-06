package compiler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"go/constant"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
)

// 同步回调的重复、结束、提交顺序和中断都必须保留真实效果。
// Synchronous callback repetition, termination, commit ordering, and interruption must preserve actual effects.
func TestCallbackPlan(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
// 中立回调载体。 A neutral callback carrier.
type Channel struct{}
// 明确的逐项输出。 Explicit item output.
func (*Channel) Emit(int){}
// 设置待提交状态。 Set a pending status.
func (*Channel) Status(int){}
// 回调调用入口。 A callback invocation entry.
func (*Channel) Run(func()bool)bool{return false}
// 允许在调用前中断。 Permit interruption before invocation.
func (*Channel) Interruptible(func()bool)bool{return false}
// 单次调用入口。 A single invocation entry.
func (*Channel) Once(func()bool)bool{return false}
// 有限重复并保留结束后的捕获修改。 Repeat finitely and preserve capture mutations after termination.
func HRepeat(c *Channel){n:=0;c.Run(func()bool{n++;c.Emit(n);return n<3});c.Emit(n+10)}
// After 效果在返回外层前提交。 After effects commit before returning to the caller.
func HCommit(c *Channel){c.Once(func()bool{c.Status(202);return false});c.Status(201);c.Emit(7)}
// 外层正常返回值进入正确分支。 The normal outer result enters the correct branch.
func HResult(c *Channel){if c.Run(func()bool{return false}){c.Emit(500)}else{c.Emit(8)}}
// 中断分支也保留外层返回值。 Interruption branches retain their outer result.
func HInterrupt(c *Channel){if c.Interruptible(func()bool{c.Emit(1);return false}){c.Emit(9)}else{c.Emit(8)}}
// 单次计划不得依返回 true 擅自循环。 A single-call plan must not repeat on a true result.
func HOnce(c *Channel){n:=0;c.Once(func()bool{n++;return true});c.Emit(n)}
// nil 回调不能生成正常文档。 A nil callback cannot produce a normal document.
func HNil(c *Channel){c.Run(nil);c.Emit(1)}
// 未知回调保留诊断。 Unknown callbacks retain diagnostics.
func HUnknown(c *Channel,f func()bool){c.Run(f);c.Emit(1)}
// 状态稳定的重复只通过中断返回，不伪造正常结束。 Stable repetition returns only through interruption, without inventing normal completion.
func HStable(c *Channel){if c.Interruptible(func()bool{c.Emit(3);return true}){c.Emit(9)}else{c.Emit(500)}}
// 不收敛的重复受预算限制。 Nonconvergent repetition remains bounded.
func HInfinite(c *Channel){c.Run(func()bool{c.Emit(1);return true})}
`
	for name, text := range map[string]string{"go.mod": "module example.test/callback-plan\n\ngo 1.27.1\n", "app.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	falseValue := core.Value{Type: types.Typ[types.Bool], Constant: constant.MakeBool(false)}
	trueValue := core.Value{Type: types.Typ[types.Bool], Constant: constant.MakeBool(true)}
	front := core.Frontend{Name: "neutral-callback-plan-v1", Match: func(f core.Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, Entry: func(f core.Function) []core.Effect {
		return []core.Effect{{Kind: core.ResponseStatus, Status: "200", Source: f.Source}}
	}, Callback: func(c core.CallContext) (*core.CallbackPlan, error) {
		if c.Object == nil {
			return nil, nil
		}
		name := c.Object.Name()
		if name != "Run" && name != "Once" && name != "Interruptible" {
			return nil, nil
		}
		plan := &core.CallbackPlan{Function: c.Arguments[0], After: []core.Effect{{Kind: core.ResponseCommit, Status: "-1", Source: c.Source}}, Results: []core.Value{falseValue}}
		if name != "Once" {
			plan.Repeat = &core.CallbackRepeat{ContinueValue: true}
		}
		if name == "Interruptible" {
			plan.MayInterrupt = true
			plan.InterruptResults = []core.Value{trueValue}
		}
		return plan, nil
	}, Call: func(c core.CallContext) ([]core.Effect, error) {
		if c.Object == nil {
			return nil, nil
		}
		if c.Object.Name() == "Status" {
			return []core.Effect{{Kind: core.ResponseStatus, Status: c.Arguments[0].Constant.ExactString(), Source: c.Source}}, nil
		}
		if c.Object.Name() != "Emit" {
			return nil, nil
		}
		if c.Arguments[0].Constant == nil {
			return nil, fmt.Errorf("item constant lost")
		}
		schema := spec.Typed("integer")
		schema.Const = spec.Set[any](json.Number(c.Arguments[0].Constant.ExactString()))
		return []core.Effect{{Kind: core.ResponseItem, Status: "-1", MediaType: "application/x-ndjson", WireSchema: schema, Source: c.Source}}, nil
	}}
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{front}, MaxCalls: 40})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Bundle.Index() {
		name := strings.TrimPrefix(entry.Symbol, "example.test/callback-plan.")
		t.Run(name, func(t *testing.T) {
			doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/items", OperationKey: entry.Key}}, openapi.Config{Title: "Callbacks", Version: "1"})
			if name == "HNil" || name == "HUnknown" || name == "HInfinite" {
				if err == nil {
					t.Fatal("unresolved callback was trusted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			status := "200"
			if name == "HCommit" {
				status = "202"
			}
			validator, err := contracttest.Compile(doc.JSON(), "/paths/~1items/get/responses/"+status+"/content/application~1x-ndjson/itemSchema", contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string][]int{"HRepeat": {1, 2, 3, 13}, "HCommit": {7}, "HResult": {8}, "HInterrupt": {1, 8, 9}, "HOnce": {1}, "HStable": {3, 9}}[name]
			for _, value := range expected {
				if err := validator.JSON([]byte(fmt.Sprint(value))); err != nil {
					t.Fatal(err)
				}
			}
			for _, value := range []int{0, 4, 10, 14, 500} {
				if validator.JSON([]byte(fmt.Sprint(value))) == nil {
					t.Fatalf("invented item %d", value)
				}
			}
		})
	}
}
