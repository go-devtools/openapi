package compiler

import (
	"go/ast"
	"go/types"
)

// A function value binds implementation and capture-cell identities; branches may share this immutable metadata.
type functionValue struct {
	object           *types.Func
	implementation   *Function
	receiver         Value
	captures         map[types.Object]uint64
	methodExpression bool
}

// Copy lexical bindings so each analysis path mutates only its own map and cell values.
func copyBindings(bindings map[types.Object]uint64) map[types.Object]uint64 {
	result := map[types.Object]uint64{}
	for object, id := range bindings {
		result[object] = id
	}
	return result
}

// Allocate independent cells for declarations and call frames while closure aliases retain the same cell identifier.
func (a *analyzer) bind(state *flow, object types.Object, value Value) {
	a.nextCell++
	state.bindings[object] = a.nextCell
	state.values[a.nextCell] = value
}

// Read a value through the current lexical frame, retaining only the type of unbound external variables.
func (s flow) read(object types.Object) Value {
	if id := s.bindings[object]; id != 0 {
		return s.values[id]
	}
	if object != nil {
		return Value{Type: object.Type()}
	}
	return Value{Unknown: true}
}

// Method values capture a value copy or an addressable variable's pointer identity according to Go rules.
func (a *analyzer) methodReceiver(fn Function, expression ast.Expr, object *types.Func, receiver Value, state *flow) Value {
	signature, _ := object.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return receiver
	}
	target := signature.Recv().Type()
	_, wantsPointer := target.Underlying().(*types.Pointer)
	_, hasPointer := receiver.Type.Underlying().(*types.Pointer)
	if wantsPointer && !hasPointer {
		if id, ok := expression.(*ast.Ident); ok {
			receiver.address = state.bindings[fn.Package.Info.ObjectOf(id)]
		}
		receiver.Type, receiver.Nil, receiver.NonNil = target, false, true
	} else if !wantsPointer && hasPointer {
		if receiver.address != 0 {
			receiver = state.values[receiver.address]
		}
		receiver.Type, receiver.address = target, 0
	}
	return receiver
}

// Distinguish unknown function values from conversions and builtins so the latter are not diagnosed as lost callbacks.
func isFunctionExpression(info *types.Info, expression ast.Expr) bool {
	if info.Types[expression].IsType() {
		return false
	}
	if id, ok := expression.(*ast.Ident); ok {
		if _, builtin := info.ObjectOf(id).(*types.Builtin); builtin {
			return false
		}
	}
	t := info.TypeOf(expression)
	if t == nil {
		return false
	}
	_, function := t.Underlying().(*types.Signature)
	return function
}

// Resolve a named function or literal closure in loaded source without executing business code.
func (a *analyzer) resolveFunction(value *functionValue, object *types.Func) (Function, bool) {
	if value != nil && value.implementation != nil {
		return *value.implementation, true
	}
	fn, ok := a.project.functions[object]
	return fn, ok && fn.Declaration != nil && fn.Declaration.Body != nil
}

// Enter an independent lexical frame, retain path-local capture cells, and restore caller bindings on return.
func (a *analyzer) invokeFunction(call CallContext, helper Function, state flow, depth int, fallback []Value) []evaluation {
	if depth >= a.options.MaxDepth {
		a.unknown(&state, call.Source, "helper calls or recursion exceed the depth budget")
		return []evaluation{{state: state, values: fallback}}
	}
	child := state.clone()
	child.bindings = map[types.Object]uint64{}
	if call.callee != nil {
		child.bindings = copyBindings(call.callee.captures)
	}
	child.ended, child.returned = false, nil
	for i := 0; i < helper.Signature.Params().Len() && i < len(call.Arguments); i++ {
		parameter := helper.Signature.Params().At(i)
		a.bind(&child, parameter, coerceValue(call.Arguments[i], parameter.Type()))
	}
	if receiver := helper.Signature.Recv(); receiver != nil {
		a.bind(&child, receiver, call.Receiver)
	}
	for i := 0; i < helper.Signature.Results().Len(); i++ {
		object := helper.Signature.Results().At(i)
		if object.Name() != "" {
			a.bind(&child, object, zeroValue(object.Type()))
		}
	}
	var results []evaluation
	for _, path := range a.statements(helper, helper.Declaration.Body.List, []flow{child}, depth+1, false) {
		returned := path.returned
		if len(returned) != len(fallback) {
			a.unknown(&path, call.Source, "helper return value is unresolved")
			returned = fallback
		}
		path.returned, path.ended, path.branch = state.returned, state.ended, state.branch
		path.bindings = copyBindings(state.bindings)
		results = append(results, evaluation{state: path, values: returned})
	}
	return a.limitEvaluations(call.Function, call.Call, results)
}
