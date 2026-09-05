package compiler

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"github.com/openapi-golang/openapi"
)

// 保存单条可达路径的值状态、效果和函数结束标志。
// Track values, effects, and termination for one reachable path.
type flow struct {
	hasCommit   bool
	headers     map[string]HeaderValue
	values      map[types.Object]Value
	effects     []Effect
	diagnostics []openapi.Diagnostic
	ended       bool
	writes      int
	pending     *Effect
	returned    []Value
	committed   string
	branch      token.Token
}

// 保存有界分析调度器，每个 handler 使用独立状态。
// Keep a bounded analyzer with independent state for each handler.
type analyzer struct {
	ctx         context.Context
	project     *Project
	options     Options
	frontend    Frontend
	calls       int
	diagnostics []openapi.Diagnostic
}

// 复制路径状态，分支不会修改另一条路径的值或效果。
// Copy path state so branches cannot mutate each other.
func (s flow) clone() flow {
	out := s
	out.headers = copyHeaders(s.headers)
	out.values = map[types.Object]Value{}
	for k, v := range s.values {
		out.values[k] = v
	}
	out.effects = append([]Effect(nil), s.effects...)
	out.diagnostics = append([]openapi.Diagnostic(nil), s.diagnostics...)
	return out
}

// 对无法可靠继续的路径保留明确诊断。
// Record a diagnostic when reliable analysis cannot continue.
func (a *analyzer) unknown(s *flow, source openapi.Source, message string) {
	s.diagnostics = append(s.diagnostics, openapi.Diagnostic{Code: "openapi.analysis.unresolved", Severity: openapi.Error, Message: message, Fix: "提供一次性集中前端规则，或收敛到预算内可分析的控制流", Source: source})
}

// 按 Go 控制流顺序传播值，已返回路径不再分析后续语句。
// Propagate values in Go execution order and stop after return.
func (a *analyzer) statements(fn Function, statements []ast.Stmt, paths []flow, depth int, handler bool) []flow {
	if depth > a.options.MaxDepth {
		for i := range paths {
			a.unknown(&paths[i], fn.Source, "跨函数深度超过预算")
		}
		return paths
	}
	for _, statement := range statements {
		if a.ctx.Err() != nil {
			for i := range paths {
				a.unknown(&paths[i], fn.Source, a.ctx.Err().Error())
			}
			return paths
		}
		var next []flow
		for _, state := range paths {
			if state.ended {
				next = append(next, state)
				continue
			}
			next = append(next, a.statement(fn, statement, state, depth, handler)...)
		}
		paths = next
		if len(paths) > a.options.MaxPaths {
			paths = paths[:a.options.MaxPaths]
			for i := range paths {
				a.unknown(&paths[i], a.project.Source(statement.Pos()), "控制流路径超过预算")
			}
			break
		}
	}
	return paths
}

// 处理单条语句，分支与 return 语义由核心管理。
// Handle statements while the core owns branching and return semantics.
func (a *analyzer) statement(fn Function, statement ast.Stmt, state flow, depth int, handler bool) []flow {
	source := a.project.Source(statement.Pos())
	switch s := statement.(type) {
	case *ast.ExprStmt:
		a.evaluate(fn, s.X, &state, depth)
	case *ast.AssignStmt:
		var values []Value
		for _, rhs := range s.Rhs {
			values = append(values, a.evaluate(fn, rhs, &state, depth))
		}
		for i, lhs := range s.Lhs {
			value := Value{Type: fn.Package.Info.TypeOf(lhs), Unknown: true}
			if i < len(values) {
				value = values[i]
			}
			a.assign(fn, lhs, value, &state)
		}
	case *ast.DeclStmt:
		if decl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range decl.Specs {
				if spec, ok := spec.(*ast.ValueSpec); ok {
					for i, name := range spec.Names {
						obj := fn.Package.Info.Defs[name]
						value := Value{Type: obj.Type()}
						if i < len(spec.Values) {
							value = a.evaluate(fn, spec.Values[i], &state, depth)
						}
						state.values[obj] = value
					}
				}
			}
		}
	case *ast.ReturnStmt:
		values := []Value{}
		for _, expr := range s.Results {
			values = append(values, a.evaluate(fn, expr, &state, depth))
		}
		state.returned = values
		if handler && a.frontend.Return != nil {
			effects, err := a.frontend.Return(ReturnContext{Function: fn, Values: values, Source: source})
			if err != nil {
				a.unknown(&state, source, err.Error())
			}
			a.effects(&state, effects)
		}
		state.ended = true
	case *ast.IfStmt:
		initial := []flow{state}
		if s.Init != nil {
			initial = a.statement(fn, s.Init, state, depth, handler)
		}
		var result []flow
		for _, start := range initial {
			condition := a.evaluate(fn, s.Cond, &start, depth)
			known := condition.Constant != nil && condition.Constant.Kind() == constant.Bool
			if !known || constant.BoolVal(condition.Constant) {
				result = append(result, a.statements(fn, s.Body.List, []flow{start.clone()}, depth, handler)...)
			}
			if !known || !constant.BoolVal(condition.Constant) {
				if s.Else != nil {
					result = append(result, a.statement(fn, s.Else, start.clone(), depth, handler)...)
				} else {
					result = append(result, start.clone())
				}
			}
		}
		return result
	case *ast.BlockStmt:
		return a.statements(fn, s.List, []flow{state}, depth, handler)
	case *ast.SwitchStmt:
		if s.Init != nil {
			state = a.statement(fn, s.Init, state, depth, handler)[0]
		}
		if s.Tag != nil {
			a.evaluate(fn, s.Tag, &state, depth)
		}
		var result []flow
		hasDefault := false
		for _, entry := range s.Body.List {
			clause := entry.(*ast.CaseClause)
			if clause.List == nil {
				hasDefault = true
			}
			result = append(result, a.statements(fn, clause.Body, []flow{state.clone()}, depth, handler)...)
		}
		if !hasDefault {
			result = append(result, state)
		}
		for i := range result {
			if result[i].branch == token.BREAK {
				result[i].ended = false
				result[i].branch = token.ILLEGAL
			}
		}
		return result
	case *ast.RangeStmt, *ast.ForStmt:
		a.unknown(&state, source, "循环中的效果需要有界摘要或集中适配")
	case *ast.GoStmt, *ast.DeferStmt:
		a.unknown(&state, source, "异步或延迟调用的响应效果需要集中适配")
	case *ast.IncDecStmt:
		a.assign(fn, s.X, Value{Type: fn.Package.Info.TypeOf(s.X), Unknown: true}, &state)
	case *ast.BranchStmt:
		if s.Tok != token.BREAK || s.Label != nil {
			a.unknown(&state, source, "未支持的控制流跳转")
		}
		state.ended = true
		state.branch = s.Tok
	case *ast.EmptyStmt:
	default:
		a.unknown(&state, source, fmt.Sprintf("未解决语句 %T", statement))
	}
	return []flow{state}
}

// 将声明和赋值关联到标准库 types 对象，不按变量名猜测身份。
// Bind assignments to types objects instead of variable-name guesses.
func (a *analyzer) assign(fn Function, lhs ast.Expr, value Value, state *flow) {
	if id, ok := lhs.(*ast.Ident); ok {
		obj := fn.Package.Info.ObjectOf(id)
		if obj != nil {
			state.values[obj] = value
		}
		return
	}
	if index, ok := lhs.(*ast.IndexExpr); ok {
		if id, ok := index.X.(*ast.Ident); ok {
			obj := fn.Package.Info.ObjectOf(id)
			old := state.values[obj]
			key := fn.Package.Info.Types[index.Index].Value
			if key != nil && key.Kind() == constant.String {
				fields := map[string]Value{}
				for k, v := range old.Fields {
					fields[k] = v
				}
				fields[constant.StringVal(key)] = value
				old.Fields = fields
				state.values[obj] = old
			} else {
				old.Unknown = true
				state.values[obj] = old
			}
		}
	}
}

// 处理写入顺序；Abort 本身不等于 Go return。
// Track write order; Abort does not imply a Go return.
func (a *analyzer) effects(state *flow, effects []Effect) {
	for _, e := range effects {
		switch e.Kind {
		case Handled:
			continue
		case ResponseHeader:
			a.responseHeader(state, e)
		case ResponseStatus, ResponseCommit:
			if state.hasCommit {
				continue
			}
			if e.Status == "-1" {
				if state.pending != nil {
					e.Status = state.pending.Status
				} else {
					e.Status = "200"
				}
			}
			if e.Kind == ResponseCommit {
				state.hasCommit = true
				state.committed = e.Status
				e.Headers = copyHeaders(state.headers)
			}
			e.Kind = ResponseStatus
			copy := e
			state.pending = &copy
		case ResponseBody:
			if state.hasCommit {
				e.Status = state.committed
			} else if e.Status == "-1" {
				if state.pending != nil {
					e.Status = state.pending.Status
				} else {
					e.Status = "200"
				}
			}
			a.checkResponseMedia(state, e)
			state.hasCommit = true
			state.committed = e.Status
			e.Headers = copyHeaders(state.headers)
			if bodylessStatus(e.Status) {
				e.Kind = ResponseStatus
				copy := e
				state.pending = &copy
				continue
			}
			state.writes++
			if state.writes > 1 {
				a.unknown(state, e.Source, "同一路径连续写入多个 body，不能表示为响应备选")
			}
			state.effects = append(state.effects, e)
			state.pending = nil
		default:
			state.effects = append(state.effects, e)
		}
	}
}

// 解析完整符号与方法选择，包括显式泛型实例。
// Resolve complete function and method identities, including generic instances.
func callObject(info *types.Info, expr ast.Expr) *types.Func {
	switch x := expr.(type) {
	case *ast.Ident:
		obj, _ := info.ObjectOf(x).(*types.Func)
		return obj
	case *ast.SelectorExpr:
		if selection := info.Selections[x]; selection != nil {
			obj, _ := selection.Obj().(*types.Func)
			return obj
		}
		obj, _ := info.ObjectOf(x.Sel).(*types.Func)
		return obj
	case *ast.IndexExpr:
		return callObject(info, x.X)
	case *ast.IndexListExpr:
		return callObject(info, x.X)
	case *ast.ParenExpr:
		return callObject(info, x.X)
	}
	return nil
}

// 求值有限常量、对象字面量、字段和可分析 helper；未知值保留明确标志。
// Evaluate bounded constants, literals, fields, and helpers while preserving unknowns.
func (a *analyzer) evaluate(fn Function, expr ast.Expr, state *flow, depth int) Value {
	info := fn.Package.Info
	value := Value{Type: info.TypeOf(expr)}
	if tv, ok := info.Types[expr]; ok && tv.Value != nil {
		value.Constant = tv.Value
		return value
	}
	switch x := expr.(type) {
	case *ast.Ident:
		if x.Name == "nil" {
			value.Nil = true
			return value
		}
		if v, ok := state.values[info.ObjectOf(x)]; ok {
			return v
		}
	case *ast.ParenExpr:
		return a.evaluate(fn, x.X, state, depth)
	case *ast.UnaryExpr:
		inner := a.evaluate(fn, x.X, state, depth)
		if x.Op == token.AND {
			inner.Type = value.Type
			return inner
		}
		if inner.Constant != nil {
			inner.Constant = constant.UnaryOp(x.Op, inner.Constant, 0)
			return inner
		}
	case *ast.StarExpr:
		inner := a.evaluate(fn, x.X, state, depth)
		inner.Type = value.Type
		return inner
	case *ast.SelectorExpr:
		base := a.evaluate(fn, x.X, state, depth)
		if field, ok := base.Fields[x.Sel.Name]; ok {
			return field
		}
	case *ast.BinaryExpr:
		left := a.evaluate(fn, x.X, state, depth)
		if left.Constant != nil && left.Constant.Kind() == constant.Bool {
			if (x.Op == token.LAND && !constant.BoolVal(left.Constant)) || (x.Op == token.LOR && constant.BoolVal(left.Constant)) {
				return left
			}
		}
		before := len(state.effects)
		right := a.evaluate(fn, x.Y, state, depth)
		if (x.Op == token.LAND || x.Op == token.LOR) && left.Constant == nil && len(state.effects) != before {
			a.unknown(state, a.project.Source(x.Pos()), "短路表达式右侧效果的适用条件尚未收敛")
		}
		if left.Constant != nil && right.Constant != nil {
			switch x.Op {
			case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
				value.Constant = constant.MakeBool(constant.Compare(left.Constant, x.Op, right.Constant))
			case token.ADD, token.SUB, token.MUL, token.QUO, token.REM, token.AND, token.OR, token.XOR, token.AND_NOT, token.LAND, token.LOR:
				value.Constant = constant.BinaryOp(left.Constant, x.Op, right.Constant)
			}
		}
		return value
	case *ast.CompositeLit:
		value.Fields = map[string]Value{}
		for i, elt := range x.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				key := ""
				if id, ok := kv.Key.(*ast.Ident); ok {
					key = id.Name
				} else {
					constantKey := info.Types[kv.Key].Value
					if constantKey != nil && constantKey.Kind() == constant.String {
						key = constant.StringVal(constantKey)
					}
				}
				if key == "" {
					value.Unknown = true
				} else {
					value.Fields[key] = a.evaluate(fn, kv.Value, state, depth)
				}
			} else if s, ok := value.Type.Underlying().(*types.Struct); ok && i < s.NumFields() {
				value.Fields[s.Field(i).Name()] = a.evaluate(fn, elt, state, depth)
			} else {
				value.Fields = nil
			}
		}
		return value
	case *ast.CallExpr:
		a.calls++
		if a.calls > a.options.MaxCalls {
			a.unknown(state, a.project.Source(expr.Pos()), "调用分析超过预算")
			value.Unknown = true
			return value
		}
		obj := callObject(info, x.Fun)
		var arguments []Value
		for _, arg := range x.Args {
			arguments = append(arguments, a.evaluate(fn, arg, state, depth))
		}
		receiver := Value{}
		if selector, ok := x.Fun.(*ast.SelectorExpr); ok {
			receiver = a.evaluate(fn, selector.X, state, depth)
		}
		call := CallContext{Function: fn, Call: x, Object: obj, Arguments: arguments, Receiver: receiver, Source: a.project.Source(x.Pos())}
		if a.frontend.Call != nil {
			effects, err := a.frontend.Call(call)
			if err != nil {
				a.unknown(state, call.Source, err.Error())
			}
			if len(effects) > 0 {
				a.effects(state, effects)
				return value
			}
		}
		if helper, ok := a.project.functions[obj]; ok && helper.Declaration.Body != nil {
			if depth >= a.options.MaxDepth {
				a.unknown(state, call.Source, "helper 或递归超过深度预算")
				value.Unknown = true
				return value
			}
			child := state.clone()
			child.ended = false
			child.returned = nil
			for i := 0; i < helper.Signature.Params().Len() && i < len(arguments); i++ {
				child.values[helper.Signature.Params().At(i)] = arguments[i]
			}
			paths := a.statements(helper, helper.Declaration.Body.List, []flow{child}, depth+1, false)
			if len(paths) == 1 {
				ended := state.ended
				*state = paths[0]
				state.ended = ended
				if len(paths[0].returned) == 1 {
					return paths[0].returned[0]
				}
			} else {
				a.unknown(state, call.Source, "多路径 helper 需要可合并的参数化摘要")
			}
			return value
		}
		if a.frontend.CarriesEffects != nil {
			for _, arg := range append(arguments, receiver) {
				if arg.Type != nil && a.frontend.CarriesEffects(arg.Type) {
					a.unknown(state, call.Source, "外部调用携带效果对象但没有已注册规则")
				}
			}
		}
		if value.Type != nil {
			if _, ok := value.Type.Underlying().(*types.Interface); ok {
				value.Unknown = true
			}
		}
		return value
	}
	if value.Type == nil {
		value.Unknown = true
	}
	return value
}
