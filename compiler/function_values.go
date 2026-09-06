package compiler

import (
	"go/ast"
	"go/types"
)

// A function value binds implementation and capture-cell identities; branches may share this immutable metadata.
// 函数值绑定实现和捕获单元身份；这些只读元数据可由分支共享。
type functionValue struct {
	object           *types.Func
	implementation   *Function
	receiver         Value
	captures         map[types.Object]uint64
	methodExpression bool
}

// Copy lexical bindings so each analysis path mutates only its own map and cell values.
// 复制词法绑定；每条分析路径只修改自己的映射与单元值。
func copyBindings(bindings map[types.Object]uint64) map[types.Object]uint64 {
	result := map[types.Object]uint64{}
	for object, id := range bindings {
		result[object] = id
	}
	return result
}

// Allocate independent cells for declarations and call frames while closure aliases retain the same cell identifier.
// 新声明和调用帧分配独立单元，闭包别名仍保留同一个单元编号。
func (a *analyzer) bind(state *flow, object types.Object, value Value) {
	a.nextCell++
	state.bindings[object] = a.nextCell
	state.values[a.nextCell] = value
}

// Read a value through the current lexical frame, retaining only the type of unbound external variables.
// 按当前词法帧读取值，未建立的外部变量保留类型但不假定其内容。
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
// 方法值按 Go 规则捕获值副本或可寻址变量的指针身份。
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
// 区分未知函数值与转换或内建函数，后两者不被误诊为丢失回调。
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
// 在已加载源码中解析命名函数或字面闭包，不执行业务代码。
func (a *analyzer) resolveFunction(value *functionValue, object *types.Func) (Function, bool) {
	if value != nil && value.implementation != nil {
		return *value.implementation, true
	}
	fn, ok := a.project.functions[object]
	return fn, ok && fn.Declaration != nil && fn.Declaration.Body != nil
}

// Enter an independent lexical frame, retain path-local capture cells, and restore caller bindings on return.
// 进入独立词法帧，保留捕获单元的路径状态，并在返回时恢复调用方绑定。
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
