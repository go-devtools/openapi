package compiler

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

// 保留一次表达式求值的路径及其单值或返回元组。
// Preserve one expression path and its single value or result tuple.
type evaluation struct {
	state  flow
	values []Value
}

// 在表达式内部同样执行路径预算，截断必须留下阻止发布的诊断。
// Enforce path budgets inside expressions and diagnose every truncated result.
func (a *analyzer) limitEvaluations(fn Function, expr ast.Node, paths []evaluation) []evaluation {
	if len(paths) <= a.options.MaxPaths {
		return paths
	}
	paths = paths[:a.options.MaxPaths]
	for i := range paths {
		a.unknown(&paths[i].state, a.project.Source(expr.Pos()), "表达式路径超过预算")
	}
	return paths
}

// 根据实际表达式类型保留未知返回元组的每一项。
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

// 只在单值上下文取值，异常元组不能伪装成一个 payload。
// Extract a scalar only in a single-value context without disguising an invalid tuple.
func scalar(e evaluation) Value {
	if len(e.values) == 1 {
		return e.values[0]
	}
	return Value{Unknown: true}
}

// 按从左到右顺序求值表达式列表，保留每条调用分支的值关联。
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

// 将已知空值与非空值转换为布尔比较；未知接口仍保留两个分支。
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

// 返回确定的布尔结果；未知条件由调用者分裂路径。
// Return a known boolean result and let callers split unknown conditions.
func knownBool(v Value) (bool, bool) {
	if v.Constant != nil && v.Constant.Kind() == constant.Bool {
		return constant.BoolVal(v.Constant), true
	}
	return false, false
}

// 求值常量、对象、短路表达式与调用，每条结果携带独立状态。
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
		if v, ok := state.values[value.Object]; ok {
			return single(v)
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
					inner.address = info.ObjectOf(id)
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
			if inner.address != nil {
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
			if field, ok := scalar(paths[i]).Fields[x.Sel.Name]; ok {
				v = field
			}
			paths[i].values = []Value{v}
		}
		return paths
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
								a.unknown(&right.state, a.project.Source(x.Pos()), "可达的除零运算")
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

// 顺序求值字面量的键和值；不能投影的集合仍保留其中调用的效果。
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

// 在实参之前求值方法接收者，并将调用结果与副作用保持在同一路径。
// Evaluate method receivers before arguments and keep call results correlated with effects.
func (a *analyzer) call(fn Function, x *ast.CallExpr, state flow, depth int) []evaluation {
	starts := []evaluation{{state: state, values: []Value{{}}}}
	if selector, ok := x.Fun.(*ast.SelectorExpr); ok && fn.Package.Info.Selections[selector] != nil {
		if fn.Package.Info.Selections[selector].Kind() == types.MethodVal {
			starts = a.evaluate(fn, selector.X, state, depth)
		}
	}
	var results []evaluation
	for _, start := range starts {
		receiver := scalar(start)
		for _, args := range a.expressions(fn, x.Args, start.state, depth) {
			call := CallContext{Response: responseSnapshot(args.state), Function: fn, Call: x, Object: callObject(fn.Package.Info, x.Fun), Arguments: args.values, Receiver: receiver, Source: a.project.Source(x.Pos())}
			results = append(results, a.invoke(call, args.state, depth)...)
		}
	}
	return a.limitEvaluations(fn, x, results)
}

// 分派已注册规则或真实 helper，有限返回备选在核心中统一传播。
// Dispatch registered rules or real helpers and propagate finite result alternatives centrally.
func (a *analyzer) invoke(call CallContext, state flow, depth int) []evaluation {
	values := expressionValues(call.Function.Package.Info.TypeOf(call.Call))
	if call.Function.Package.Info.Types[call.Call.Fun].IsType() && len(call.Arguments) == 1 && len(values) == 1 {
		v := coerceValue(call.Arguments[0], values[0].Type)
		if _, boxed := values[0].Type.Underlying().(*types.Interface); !boxed {
			v.Type = values[0].Type
		}
		return []evaluation{{state: state, values: []Value{v}}}
	}
	fallback := func() []evaluation { return []evaluation{{state: state, values: values}} }
	a.calls++
	if a.calls > a.options.MaxCalls {
		a.unknown(&state, call.Source, "调用分析超过预算")
		return fallback()
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
				a.unknown(&state, call.Source, "调用返回备选超过路径预算")
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
					a.unknown(&branch, call.Source, "前端返回备选的结果数量与 Go 签名不一致")
					results = append(results, evaluation{state: branch, values: values})
					continue
				}
				returned := append([]Value(nil), outcome.Results...)
				for i := range returned {
					if returned[i].Type == nil {
						returned[i].Type = values[i].Type
					}
					if returned[i].Nil && returned[i].NonNil {
						a.unknown(&branch, call.Source, "前端返回备选同时声明 nil 和非 nil")
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
	if helper, ok := a.project.functions[call.Object]; ok && helper.Declaration.Body != nil {
		if depth >= a.options.MaxDepth {
			a.unknown(&state, call.Source, "helper 或递归超过深度预算")
			return fallback()
		}
		child := state.clone()
		child.ended = false
		child.returned = nil
		for i := 0; i < helper.Signature.Params().Len() && i < len(call.Arguments); i++ {
			child.values[helper.Signature.Params().At(i)] = call.Arguments[i]
		}
		if helper.Signature.Recv() != nil {
			child.values[helper.Signature.Recv()] = call.Receiver
		}
		for i := 0; i < helper.Signature.Results().Len(); i++ {
			obj := helper.Signature.Results().At(i)
			if obj.Name() != "" {
				child.values[obj] = zeroValue(obj.Type())
			}
		}
		var results []evaluation
		for _, path := range a.statements(helper, helper.Declaration.Body.List, []flow{child}, depth+1, false) {
			returned := path.returned
			if len(returned) != len(values) {
				a.unknown(&path, call.Source, "helper 返回值尚未解决")
				returned = values
			}
			path.returned = state.returned
			path.ended = state.ended
			path.branch = state.branch
			results = append(results, evaluation{state: path, values: returned})
		}
		return a.limitEvaluations(call.Function, call.Call, results)
	}
	invalidateAddresses(&state, call.Arguments)
	if a.frontend.CarriesEffects != nil {
		for _, arg := range append(append([]Value(nil), call.Arguments...), call.Receiver) {
			if arg.Type != nil && a.frontend.CarriesEffects(arg.Type) {
				a.unknown(&state, call.Source, "外部调用携带效果对象但没有已注册规则")
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

// 初始化 Go 零值，用于无初始化表达式的声明和具名返回值。
// Initialize Go zero values for declarations without expressions and named results.
func zeroValue(t types.Type) Value {
	value := Value{Type: t}
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

// 接口装箱不能把有动态类型的 nil 指针误认为 nil 接口。
// Interface boxing must not confuse a typed nil pointer with a nil interface.
func coerceValue(value Value, target types.Type) Value {
	if target == nil {
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

// 外部调用可能修改传入地址，旧常量和 nil 事实不能继续用于分支裁剪。
// External calls may mutate passed addresses, invalidating old constants and nil facts used for pruning.
func invalidateAddresses(state *flow, arguments []Value) {
	for _, arg := range arguments {
		if arg.address != nil {
			old := state.values[arg.address]
			state.values[arg.address] = Value{Type: old.Type}
		}
	}
}
