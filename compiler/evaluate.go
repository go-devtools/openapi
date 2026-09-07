package compiler

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

// Preserve one expression path and its single value or result tuple.
type evaluation struct {
	state  flow
	values []Value
}

// Enforce path budgets inside expressions and diagnose every truncated result.
func (a *analyzer) limitEvaluations(fn Function, expr ast.Node, paths []evaluation) []evaluation {
	if len(paths) <= a.options.MaxPaths {
		return paths
	}
	paths = paths[:a.options.MaxPaths]
	for i := range paths {
		a.unknown(&paths[i].state, a.project.Source(expr.Pos()), "expression paths exceed the budget")
	}
	return paths
}

// Preserve each unknown result using the expression's actual tuple type.
func expressionValues(t types.Type) []Value {
	if tuple, ok := t.(*types.Tuple); ok {
		values := make([]Value, tuple.Len())
		for i := range values {
			values[i] = Value{Type: tuple.At(i).Type()}
		}
		return values
	}
	if t == nil {
		return nil
	}
	return []Value{{Type: t}}
}

// Extract a scalar only in a single-value context without disguising an invalid tuple.
func scalar(e evaluation) Value {
	if len(e.values) == 1 {
		return e.values[0]
	}
	return Value{Unknown: true}
}

// Evaluate expression lists from left to right while preserving path-value correlation.
func (a *analyzer) expressions(fn Function, exprs []ast.Expr, state flow, depth int) []evaluation {
	paths := []evaluation{{state: state}}
	for _, expr := range exprs {
		var next []evaluation
		for _, path := range paths {
			for _, result := range a.evaluate(fn, expr, path.state, depth) {
				result.values = append(append([]Value(nil), path.values...), result.values...)
				next = append(next, result)
			}
		}
		paths = a.limitEvaluations(fn, expr, next)
	}
	return paths
}

// Compare known nil and non-nil values while keeping unknown interfaces undecided.
func compareValues(left Value, op token.Token, right Value) Value {
	value := Value{Type: types.Typ[types.Bool]}
	if left.Constant != nil && right.Constant != nil {
		value.Constant = constant.MakeBool(constant.Compare(left.Constant, op, right.Constant))
	} else if op == token.EQL || op == token.NEQ {
		known, equal := false, false
		if left.Nil && right.Nil {
			known, equal = true, true
		}
		if left.Nil && right.NonNil || left.NonNil && right.Nil {
			known = true
		}
		if known {
			if op == token.NEQ {
				equal = !equal
			}
			value.Constant = constant.MakeBool(equal)
		}
	}
	return value
}

// Return a known boolean result and let callers split unknown conditions.
func knownBool(v Value) (bool, bool) {
	if v.Constant != nil && v.Constant.Kind() == constant.Bool {
		return constant.BoolVal(v.Constant), true
	}
	return false, false
}

// Evaluate constants, objects, short-circuit expressions, and calls with independent result states.
func (a *analyzer) evaluate(fn Function, expr ast.Expr, state flow, depth int) []evaluation {
	info := fn.Package.Info
	value := Value{Type: info.TypeOf(expr)}
	single := func(v Value) []evaluation { return []evaluation{{state: state, values: []Value{v}}} }
	if tv, ok := info.Types[expr]; ok && tv.Value != nil {
		value.Constant = tv.Value
		return single(value)
	}
	switch x := expr.(type) {
	case *ast.Ident:
		if x.Name == "nil" {
			value.Nil = true
			return single(value)
		}
		value.Object = info.ObjectOf(x)
		if id := state.bindings[value.Object]; id != 0 {
			return single(state.values[id])
		}
		if object, ok := value.Object.(*types.Func); ok {
			value.callable = &functionValue{object: object}
			value.NonNil = true
		}
	case *ast.ParenExpr:
		return a.evaluate(fn, x.X, state, depth)
	case *ast.UnaryExpr:
		paths := a.evaluate(fn, x.X, state, depth)
		for i := range paths {
			inner := scalar(paths[i])
			inner.Type = value.Type
			if x.Op == token.AND {
				inner.Boxed, inner.DynamicNil, inner.DynamicNonNil = false, false, false
				inner.Nil = false
				inner.NonNil = true
				if id, ok := x.X.(*ast.Ident); ok {
					inner.address = paths[i].state.bindings[info.ObjectOf(id)]
				}
			} else if inner.Constant != nil {
				inner.Constant = constant.UnaryOp(x.Op, inner.Constant, 0)
			}
			paths[i].values = []Value{inner}
		}
		return paths
	case *ast.StarExpr:
		paths := a.evaluate(fn, x.X, state, depth)
		for i := range paths {
			inner := scalar(paths[i])
			if inner.address != 0 {
				paths[i].values = []Value{paths[i].state.values[inner.address]}
				continue
			}
			inner.Type = value.Type
			inner.Nil = false
			inner.NonNil = false
			paths[i].values = []Value{inner}
		}
		return paths
	case *ast.SelectorExpr:
		value.Object = info.ObjectOf(x.Sel)
		paths := a.evaluate(fn, x.X, state, depth)
		for i := range paths {
			v := value
			base := scalar(paths[i])
			if object, ok := value.Object.(*types.Func); ok {
				v.callable = &functionValue{object: object}
				v.NonNil = true
				if selection := info.Selections[x]; selection != nil {
					v.callable.methodExpression = selection.Kind() == types.MethodExpr
					if !v.callable.methodExpression {
						v.callable.receiver = a.methodReceiver(fn, x.X, object, base, &paths[i].state)
					}
				}
			} else {
				if base.address != 0 {
					base = paths[i].state.values[base.address]
				}
				if field, ok := base.Fields[x.Sel.Name]; ok {
					v = field
				}
			}
			paths[i].values = []Value{v}
		}
		return paths
	case *ast.FuncLit:
		signature, _ := value.Type.Underlying().(*types.Signature)
		implementation := Function{Signature: signature, Declaration: &ast.FuncDecl{Type: x.Type, Body: x.Body}, Package: fn.Package, Source: a.project.Source(x.Pos())}
		value.callable = &functionValue{implementation: &implementation, captures: copyBindings(state.bindings)}
		value.NonNil = true
		return single(value)
	case *ast.BinaryExpr:
		var out []evaluation
		for _, left := range a.evaluate(fn, x.X, state, depth) {
			lv := scalar(left)
			if x.Op == token.LAND || x.Op == token.LOR {
				b, known := knownBool(lv)
				stop := x.Op == token.LOR
				if !known || b == stop {
					out = append(out, evaluation{state: left.state.clone(), values: []Value{{Type: value.Type, Constant: constant.MakeBool(stop)}}})
				}
				if known && b == stop {
					continue
				}
				for _, right := range a.evaluate(fn, x.Y, left.state, depth) {
					out = append(out, right)
				}
			} else {
				for _, right := range a.evaluate(fn, x.Y, left.state, depth) {
					rv := scalar(right)
					v := value
					switch x.Op {
					case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
						v = compareValues(lv, x.Op, rv)
					case token.ADD, token.SUB, token.MUL, token.QUO, token.REM, token.AND, token.OR, token.XOR, token.AND_NOT:
						if lv.Constant != nil && rv.Constant != nil {
							if (x.Op == token.QUO || x.Op == token.REM) && constant.Sign(rv.Constant) == 0 {
								a.unknown(&right.state, a.project.Source(x.Pos()), "reachable division by zero")
							} else {
								v.Constant = constant.BinaryOp(lv.Constant, x.Op, rv.Constant)
							}
						}
					}
					right.values = []Value{v}
					out = append(out, right)
				}
			}
		}
		return a.limitEvaluations(fn, x, out)
	case *ast.CompositeLit:
		return a.composite(fn, x, state, depth)
	case *ast.CallExpr:
		return a.call(fn, x, state, depth)
	case *ast.IndexExpr:
		paths := a.expressions(fn, []ast.Expr{x.X, x.Index}, state, depth)
		for i := range paths {
			v := value
			if len(paths[i].values) == 2 {
				base, key := paths[i].values[0], paths[i].values[1]
				if key.Constant != nil && key.Constant.Kind() == constant.String {
					if field, ok := base.Fields[constant.StringVal(key.Constant)]; ok {
						v = field
					}
				}
			}
			paths[i].values = []Value{v}
		}
		return paths
	}
	if value.Type == nil {
		value.Unknown = true
	}
	return single(value)
}

// Evaluate literal keys and values in order, retaining call effects even for opaque collections.
func (a *analyzer) composite(fn Function, x *ast.CompositeLit, state flow, depth int) []evaluation {
	value := Value{Type: fn.Package.Info.TypeOf(x), Fields: map[string]Value{}}
	paths := []evaluation{{state: state, values: []Value{value}}}
	for index, element := range x.Elts {
		key := ""
		expr := element
		var keyExpr ast.Expr
		if kv, ok := element.(*ast.KeyValueExpr); ok {
			expr = kv.Value
			if _, ok := value.Type.Underlying().(*types.Struct); ok {
				if id, ok := kv.Key.(*ast.Ident); ok {
					key = id.Name
				}
			} else {
				keyExpr = kv.Key
				if k := fn.Package.Info.Types[kv.Key].Value; k != nil && k.Kind() == constant.String {
					key = constant.StringVal(k)
				}
			}
		} else if s, ok := value.Type.Underlying().(*types.Struct); ok && index < s.NumFields() {
			key = s.Field(index).Name()
		}
		var next []evaluation
		for _, path := range paths {
			start := []evaluation{{state: path.state}}
			if keyExpr != nil {
				start = a.evaluate(fn, keyExpr, path.state, depth)
			}
			for _, initial := range start {
				for _, item := range a.evaluate(fn, expr, initial.state, depth) {
					v := scalar(path)
					fields := map[string]Value{}
					for k, f := range v.Fields {
						fields[k] = f
					}
					v.Fields = fields
					if key != "" {
						v.Fields[key] = scalar(item)
					} else {
						v.Fields = nil
					}
					item.values = []Value{v}
					next = append(next, item)
				}
			}
		}
		paths = a.limitEvaluations(fn, x, next)
	}
	return paths
}

// Evaluate method receivers before arguments and keep call results correlated with effects.
func (a *analyzer) call(fn Function, x *ast.CallExpr, state flow, depth int) []evaluation {
	var results []evaluation
	for _, start := range a.evaluate(fn, x.Fun, state, depth) {
		callee := scalar(start)
		for _, args := range a.expressions(fn, x.Args, start.state, depth) {
			call := CallContext{Response: responseSnapshot(args.state), Function: fn, Call: x, Object: callObject(fn.Package.Info, x.Fun), Arguments: args.values, Source: a.project.Source(x.Pos()), callee: callee.callable}
			if call.callee != nil {
				call.Object = call.callee.object
				call.Receiver = call.callee.receiver
				if call.callee.methodExpression && len(call.Arguments) > 0 {
					call.Receiver, call.Arguments = call.Arguments[0], call.Arguments[1:]
				}
			}
			results = append(results, a.invoke(call, args.state, depth)...)
		}
	}
	return a.limitEvaluations(fn, x, results)
}

// Dispatch registered rules or real helpers and propagate finite result alternatives centrally.
func (a *analyzer) invoke(call CallContext, state flow, depth int) []evaluation {
	values := expressionValues(call.Function.Package.Info.TypeOf(call.Call))
	if call.Function.Package.Info.Types[call.Call.Fun].IsType() && len(call.Arguments) == 1 && len(values) == 1 {
		v := coerceValue(call.Arguments[0], values[0].Type)
		// A conversion to T changes type identity; only an actual interface conversion preserves a concrete boxed type.
		_, parameter := types.Unalias(values[0].Type).(*types.TypeParam)
		if _, boxed := values[0].Type.Underlying().(*types.Interface); !boxed || parameter {
			v.Type = values[0].Type
		}
		return []evaluation{{state: state, values: []Value{v}}}
	}
	fallback := func() []evaluation { return []evaluation{{state: state, values: values}} }
	a.calls++
	if a.calls > a.options.MaxCalls {
		a.unknown(&state, call.Source, "call analysis exceeds the budget")
		return fallback()
	}
	if a.frontend.Callback != nil {
		plan, err := a.frontend.Callback(call)
		if err != nil {
			a.unknown(&state, call.Source, err.Error())
			return fallback()
		}
		if plan != nil {
			return a.invokeCallback(call, *plan, state, depth, values)
		}
	}
	if a.frontend.CallOutcomes != nil {
		outcomes, err := a.frontend.CallOutcomes(call)
		if err != nil {
			a.unknown(&state, call.Source, err.Error())
			return fallback()
		}
		if len(outcomes) > 0 {
			invalidateAddresses(&state, call.Arguments)
			if len(outcomes) > a.options.MaxPaths {
				outcomes = outcomes[:a.options.MaxPaths]
				a.unknown(&state, call.Source, "call return alternatives exceed the path budget")
			}
			var results []evaluation
			for _, outcome := range outcomes {
				branch := state.clone()
				when, reachable, err := state.when.Intersect(outcome.When)
				if err != nil {
					a.unknown(&branch, call.Source, err.Error())
					results = append(results, evaluation{state: branch, values: values})
					continue
				}
				if !reachable {
					continue
				}
				branch.when = when
				if len(outcome.Results) != len(values) {
					a.unknown(&branch, call.Source, "frontend return alternative count differs from the Go signature")
					results = append(results, evaluation{state: branch, values: values})
					continue
				}
				returned := append([]Value(nil), outcome.Results...)
				for i := range returned {
					if returned[i].Type == nil {
						returned[i].Type = values[i].Type
					}
					if returned[i].Nil && returned[i].NonNil {
						a.unknown(&branch, call.Source, "frontend return alternative declares both nil and non-nil")
					}
				}
				a.effects(&branch, outcome.Effects)
				results = append(results, evaluation{state: branch, values: returned})
			}
			return a.limitEvaluations(call.Function, call.Call, results)
		}
	}
	if a.frontend.Call != nil {
		effects, err := a.frontend.Call(call)
		if err != nil {
			a.unknown(&state, call.Source, err.Error())
		}
		if len(effects) > 0 {
			invalidateAddresses(&state, call.Arguments)
			a.effects(&state, effects)
			return fallback()
		}
	}
	if helper, ok := a.resolveFunction(call.callee, call.Object); ok {
		return a.invokeFunction(call, helper, state, depth, values)
	}
	if call.Object == nil && isFunctionExpression(call.Function.Package.Info, call.Call.Fun) {
		a.unknown(&state, call.Source, "function value is nil or its actual implementation is unresolved; call effects cannot be ignored")
	}
	invalidateAddresses(&state, call.Arguments)
	if a.frontend.CarriesEffects != nil {
		for _, arg := range append(append([]Value(nil), call.Arguments...), call.Receiver) {
			if arg.Type != nil && a.frontend.CarriesEffects(arg.Type) {
				a.unknown(&state, call.Source, "external call carries an effect object but has no registered rule")
			}
		}
	}
	for i := range values {
		if _, ok := values[i].Type.Underlying().(*types.Interface); ok {
			values[i].Unknown = true
		}
	}
	return fallback()
}

// Initialize Go zero values for declarations without expressions and named results.
func zeroValue(t types.Type) Value {
	value := Value{Type: t}
	// The underlying constraint interface says nothing about an uninstantiated parameter's zero value.
	if _, parameter := types.Unalias(t).(*types.TypeParam); parameter {
		value.Unknown = true
		return value
	}
	switch typ := t.Underlying().(type) {
	case *types.Struct:
		value.Fields = map[string]Value{}
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			value.Fields[field.Name()] = zeroValue(field.Type())
		}
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Signature, *types.Chan:
		value.Nil = true
	case *types.Basic:
		switch {
		case typ.Info()&types.IsBoolean != 0:
			value.Constant = constant.MakeBool(false)
		case typ.Info()&types.IsString != 0:
			value.Constant = constant.MakeString("")
		case typ.Info()&types.IsNumeric != 0:
			value.Constant = constant.MakeInt64(0)
		}
	}
	return value
}

// Interface boxing must not confuse a typed nil pointer with a nil interface.
func coerceValue(value Value, target types.Type) Value {
	if target == nil {
		return value
	}
	// Passing a concrete argument to a generic parameter does not box it into the constraint interface.
	if _, parameter := types.Unalias(target).(*types.TypeParam); parameter {
		return value
	}
	if value.Type == nil || types.Identical(value.Type, types.Typ[types.UntypedNil]) {
		value.Type = target
		return value
	}
	if _, ok := target.Underlying().(*types.Interface); ok {
		if _, already := value.Type.Underlying().(*types.Interface); !already {
			if !value.Boxed {
				value.DynamicNil, value.DynamicNonNil = value.Nil, value.NonNil
				value.Boxed = true
			}
			value.Nil = false
			value.NonNil = true
		}
	}
	return value
}

// External calls may mutate passed addresses, invalidating old constants and nil facts used for pruning.
func invalidateAddresses(state *flow, arguments []Value) {
	for _, arg := range arguments {
		if arg.address != 0 {
			old := state.values[arg.address]
			state.values[arg.address] = Value{Type: old.Type}
		}
	}
}
