package compiler

import (
	"go/types"
	"reflect"
)

// Describe a synchronous callback or repetition controlled by its boolean result without framework types.
type CallbackPlan struct {
	Function  Value
	Arguments []Value
	After     []Effect
	Results   []Value
	Repeat    *CallbackRepeat
	// Preserve possible interruption before invocation and the corresponding outer results.
	MayInterrupt     bool
	InterruptResults []Value
}

// Select the boolean result index and continuation value; public compile options set the iteration budget.
type CallbackRepeat struct {
	ResultIndex   int
	ContinueValue bool
}

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

// Invoke callbacks through shared lexical state, bounding repetition and interruption with the same call and path budgets.
func (a *analyzer) invokeCallback(call CallContext, plan CallbackPlan, state flow, depth int, fallback []Value) []evaluation {
	fail := func(message string) []evaluation {
		a.unknown(&state, call.Source, message)
		return []evaluation{{state: state, values: fallback}}
	}
	normal, ok := callbackResults(plan.Results, fallback)
	if !ok {
		return fail("callback plan outer return tuple differs from the Go signature")
	}
	interrupted, ok := callbackResults(plan.InterruptResults, fallback)
	if plan.MayInterrupt && !ok {
		return fail("interrupted callback return tuple differs from the Go signature")
	}
	callable := plan.Function.callable
	if callable == nil || plan.Function.Nil || plan.Function.DynamicNil {
		return fail("callback is nil or its actual function implementation is unresolved")
	}
	helper, ok := a.resolveFunction(callable, callable.object)
	if !ok {
		return fail("callback has no analyzable source implementation")
	}
	callbackCall := call
	callbackCall.callee, callbackCall.Object, callbackCall.Receiver, callbackCall.Arguments = callable, callable.object, callable.receiver, plan.Arguments
	var err error
	helper, err = a.instantiateFunction(callbackCall, helper)
	if err != nil {
		return fail(err.Error())
	}
	signature := helper.Signature
	if signature.Variadic() || len(plan.Arguments) != signature.Params().Len() {
		return fail("callback arguments do not match or variadic expansion is unresolved")
	}
	for i, value := range plan.Arguments {
		if value.Type == nil || !types.AssignableTo(value.Type, helper.concrete(signature.Params().At(i).Type())) {
			return fail("callback argument types differ from the Go signature")
		}
	}
	var callbackReturns []Value
	for i := 0; i < signature.Results().Len(); i++ {
		callbackReturns = append(callbackReturns, Value{Type: helper.concrete(signature.Results().At(i).Type())})
	}
	if plan.Repeat != nil {
		index := plan.Repeat.ResultIndex
		if index < 0 || index >= len(callbackReturns) {
			return fail("callback repetition condition references a nonexistent return position")
		}
		basic, ok := callbackReturns[index].Type.Underlying().(*types.Basic)
		if !ok || basic.Info()&types.IsBoolean == 0 {
			return fail("callback repetition condition requires a boolean result")
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
				a.unknown(&before, call.Source, "synchronous callback repetition exceeded the budget or was canceled")
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
