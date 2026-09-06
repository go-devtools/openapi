package compiler

import (
	"go/types"
	"reflect"
)

// 描述一次同步回调或由其布尔返回值控制的重复调用，不包含框架类型。
// Describe a synchronous callback or repetition controlled by its boolean result without framework types.
type CallbackPlan struct {
	Function  Value
	Arguments []Value
	After     []Effect
	Results   []Value
	Repeat    *CallbackRepeat
	// 调用前可能中断，保留此路径和对应外层返回值。
	// Preserve possible interruption before invocation and the corresponding outer results.
	MayInterrupt     bool
	InterruptResults []Value
}

// 指定控制重复的布尔返回位置及继续值；次数预算在公共编译选项中设置。
// Select the boolean result index and continuation value; public compile options set the iteration budget.
type CallbackRepeat struct {
	ResultIndex   int
	ContinueValue bool
}

// 校验外层返回元组并保留其实际 Go 类型，不接受错误的前端返回约定。
// Validate the outer result tuple and preserve actual Go types, rejecting incorrect frontend result conventions.
func callbackResults(values, expected []Value) ([]Value, bool) {
	if len(values) != len(expected) {
		return nil, false
	}
	result := append([]Value(nil), values...)
	for i := range result {
		if result[i].Type == nil {
			result[i].Type = expected[i].Type
		}
		if !types.AssignableTo(result[i].Type, expected[i].Type) || result[i].Nil && result[i].NonNil {
			return nil, false
		}
		result[i] = coerceValue(result[i], expected[i].Type)
	}
	return result, true
}

// 使用共享词法状态逐次调用回调；重复和中断均受同一调用及路径预算约束。
// Invoke callbacks through shared lexical state, bounding repetition and interruption with the same call and path budgets.
func (a *analyzer) invokeCallback(call CallContext, plan CallbackPlan, state flow, depth int, fallback []Value) []evaluation {
	fail := func(message string) []evaluation {
		a.unknown(&state, call.Source, message)
		return []evaluation{{state: state, values: fallback}}
	}
	normal, ok := callbackResults(plan.Results, fallback)
	if !ok {
		return fail("回调计划的外层返回元组与 Go 签名不一致")
	}
	interrupted, ok := callbackResults(plan.InterruptResults, fallback)
	if plan.MayInterrupt && !ok {
		return fail("回调中断的返回元组与 Go 签名不一致")
	}
	callable := plan.Function.callable
	if callable == nil || plan.Function.Nil || plan.Function.DynamicNil {
		return fail("回调为 nil 或实际函数实现未解决")
	}
	helper, ok := a.resolveFunction(callable, callable.object)
	if !ok {
		return fail("回调没有可分析的源码实现")
	}
	signature := helper.Signature
	if signature.Variadic() || len(plan.Arguments) != signature.Params().Len() {
		return fail("回调实参不匹配或变参展开规则未解决")
	}
	for i, value := range plan.Arguments {
		if value.Type == nil || !types.AssignableTo(value.Type, signature.Params().At(i).Type()) {
			return fail("回调实参类型与 Go 签名不一致")
		}
	}
	var callbackReturns []Value
	for i := 0; i < signature.Results().Len(); i++ {
		callbackReturns = append(callbackReturns, Value{Type: signature.Results().At(i).Type()})
	}
	if plan.Repeat != nil {
		index := plan.Repeat.ResultIndex
		if index < 0 || index >= len(callbackReturns) {
			return fail("回调重复条件引用了不存在的返回位置")
		}
		basic, ok := callbackReturns[index].Type.Underlying().(*types.Basic)
		if !ok || basic.Info()&types.IsBoolean == 0 {
			return fail("回调重复条件必须使用布尔返回值")
		}
	}
	pending := []flow{state}
	var finished []evaluation
	for iteration := 0; len(pending) > 0; iteration++ {
		var next []flow
		for _, before := range pending {
			if plan.MayInterrupt {
				finished = append(finished, evaluation{state: before.clone(), values: interrupted})
			}
			if iteration >= a.options.MaxIterations || a.calls >= a.options.MaxCalls || a.ctx.Err() != nil {
				a.unknown(&before, call.Source, "同步回调重复超过预算或已取消")
				finished = append(finished, evaluation{state: before, values: normal})
				continue
			}
			a.calls++
			callbackCall := call
			callbackCall.callee, callbackCall.Object, callbackCall.Receiver, callbackCall.Arguments = callable, callable.object, callable.receiver, plan.Arguments
			for _, after := range a.invokeFunction(callbackCall, helper, before, depth, callbackReturns) {
				a.effects(&after.state, plan.After)
				if plan.Repeat == nil {
					finished = append(finished, evaluation{state: after.state, values: normal})
					continue
				}
				value, known := knownBool(after.values[plan.Repeat.ResultIndex])
				if !known || value != plan.Repeat.ContinueValue {
					finished = append(finished, evaluation{state: after.state.clone(), values: normal})
				}
				if !known || value == plan.Repeat.ContinueValue {
					if len(after.state.diagnostics) > len(before.diagnostics) {
						finished = append(finished, evaluation{state: after.state, values: normal})
					} else if (plan.MayInterrupt || !known) && sameCallbackState(before, after.state, plan) {
						// 同一抽象状态的下一次调用不会产生新的契约；只能保留已经存在的出口。
						// Reinvoking the same abstract state adds no contract; retain only exits that already exist.
						if plan.MayInterrupt {
							finished = append(finished, evaluation{state: after.state, values: interrupted})
						}
					} else {
						next = append(next, after.state)
					}
				}
			}
		}
		if len(next)+len(finished) > a.options.MaxPaths {
			for _, path := range next {
				finished = append(finished, evaluation{state: path, values: normal})
			}
			finished = a.limitEvaluations(call.Function, call.Call, finished)
			break
		}
		pending = next
	}
	return a.limitEvaluations(call.Function, call.Call, finished)
}

// 只比较可达捕获单元和响应状态，已退出帧的临时变量不阻止固定点收敛。
// Compare reachable captured cells and response state; temporaries from exited frames do not prevent fixed-point convergence.
func sameCallbackState(before, after flow, plan CallbackPlan) bool {
	if before.bodyKind != after.bodyKind || before.bodyMedia != after.bodyMedia || min(before.writes, 2) != min(after.writes, 2) || !reflect.DeepEqual(before.when, after.when) || !reflect.DeepEqual(responseSnapshot(before), responseSnapshot(after)) {
		return false
	}
	snapshots := func(state flow) (map[uint64]Value, bool) {
		cells := map[uint64]Value{}
		valid := true
		var visit func(Value, int)
		var cell func(uint64, int)
		cell = func(id uint64, depth int) {
			if id == 0 {
				return
			}
			if _, seen := cells[id]; seen {
				return
			}
			value, ok := state.values[id]
			if !ok {
				valid = false
				return
			}
			cells[id] = value
			visit(value, depth+1)
		}
		visit = func(value Value, depth int) {
			if depth > 128 {
				valid = false
				return
			}
			cell(value.address, depth)
			if value.callable != nil {
				for _, id := range value.callable.captures {
					cell(id, depth)
				}
				visit(value.callable.receiver, depth+1)
			}
			for _, field := range value.Fields {
				visit(field, depth+1)
			}
		}
		for _, id := range state.bindings {
			cell(id, 0)
		}
		visit(plan.Function, 0)
		for _, argument := range plan.Arguments {
			visit(argument, 0)
		}
		return cells, valid
	}
	left, leftOK := snapshots(before)
	right, rightOK := snapshots(after)
	return leftOK && rightOK && reflect.DeepEqual(left, right)
}
