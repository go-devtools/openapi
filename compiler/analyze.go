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
	when        openapi.RequestCondition
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

// 处理单条语句，表达式备选保持独立直到下一条语句。
// Handle each statement while keeping expression alternatives independent through subsequent statements.
func (a *analyzer) statement(fn Function, statement ast.Stmt, state flow, depth int, handler bool) []flow {
	source := a.project.Source(statement.Pos())
	switch s := statement.(type) {
	case *ast.ExprStmt:
		var result []flow
		for _, e := range a.evaluate(fn, s.X, state, depth) {
			result = append(result, e.state)
		}
		return result
	case *ast.AssignStmt:
		var result []flow
		for _, e := range a.expressions(fn, s.Rhs, state, depth) {
			for i, lhs := range s.Lhs {
				value := Value{Type: fn.Package.Info.TypeOf(lhs), Unknown: true}
				if i < len(e.values) {
					value = e.values[i]
				}
				if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
					a.unknown(&e.state, source, "复合赋值需要明确运算传播")
					value = Value{Type: fn.Package.Info.TypeOf(lhs), Unknown: true}
				}
				a.assign(fn, lhs, value, &e.state)
			}
			result = append(result, e.state)
		}
		return result
	case *ast.DeclStmt:
		paths := []flow{state}
		if decl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, entry := range decl.Specs {
				if spec, ok := entry.(*ast.ValueSpec); ok {
					var next []flow
					for _, path := range paths {
						for _, e := range a.expressions(fn, spec.Values, path, depth) {
							for i, name := range spec.Names {
								obj := fn.Package.Info.Defs[name]
								value := zeroValue(obj.Type())
								if i < len(e.values) {
									value = e.values[i]
								}
								e.state.values[obj] = coerceValue(value, obj.Type())
							}
							next = append(next, e.state)
						}
					}
					paths = next
				}
			}
		}
		return paths
	case *ast.ReturnStmt:
		var result []flow
		for _, e := range a.expressions(fn, s.Results, state, depth) {
			if len(s.Results) == 0 {
				for i := 0; i < fn.Signature.Results().Len(); i++ {
					obj := fn.Signature.Results().At(i)
					e.values = append(e.values, e.state.values[obj])
				}
			}
			for i := range e.values {
				if i < fn.Signature.Results().Len() {
					e.values[i] = coerceValue(e.values[i], fn.Signature.Results().At(i).Type())
				}
			}
			e.state.returned = e.values
			if handler && a.frontend.Return != nil {
				effects, err := a.frontend.Return(ReturnContext{Function: fn, Values: e.values, Source: source})
				if err != nil {
					a.unknown(&e.state, source, err.Error())
				}
				a.effects(&e.state, effects)
			}
			e.state.ended = true
			result = append(result, e.state)
		}
		return result
	case *ast.IfStmt:
		initial := []flow{state}
		if s.Init != nil {
			initial = a.statement(fn, s.Init, state, depth, handler)
		}
		var result []flow
		for _, start := range initial {
			for _, condition := range a.evaluate(fn, s.Cond, start, depth) {
				b, known := knownBool(scalar(condition))
				if !known || b {
					result = append(result, a.statements(fn, s.Body.List, []flow{condition.state.clone()}, depth, handler)...)
				}
				if !known || !b {
					if s.Else != nil {
						result = append(result, a.statement(fn, s.Else, condition.state.clone(), depth, handler)...)
					} else {
						result = append(result, condition.state.clone())
					}
				}
			}
		}
		return result
	case *ast.BlockStmt:
		return a.statements(fn, s.List, []flow{state}, depth, handler)
	case *ast.SwitchStmt:
		return a.switchStatement(fn, s, state, depth, handler)
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

// 按 Go 顺序检查 switch 标签，只将尚未匹配的路径传给下一分支。
// Check switch cases in Go order and pass only unmatched paths to the next case.
func (a *analyzer) switchStatement(fn Function, s *ast.SwitchStmt, state flow, depth int, handler bool) []flow {
	initial := []flow{state}
	if s.Init != nil {
		initial = a.statement(fn, s.Init, state, depth, handler)
	}
	var pending []evaluation
	for _, path := range initial {
		if s.Tag != nil {
			pending = append(pending, a.evaluate(fn, s.Tag, path, depth)...)
		} else {
			pending = append(pending, evaluation{state: path, values: []Value{{Type: types.Typ[types.Bool], Constant: constant.MakeBool(true)}}})
		}
	}
	var result []flow
	var defaultBody []ast.Stmt
	for _, entry := range s.Body.List {
		clause := entry.(*ast.CaseClause)
		if clause.List == nil {
			defaultBody = clause.Body
			continue
		}
		var matches []flow
		for _, label := range clause.List {
			var remaining []evaluation
			for _, path := range pending {
				tag := scalar(path)
				for _, test := range a.evaluate(fn, label, path.state, depth) {
					b, known := knownBool(compareValues(tag, token.EQL, scalar(test)))
					if !known || b {
						matches = append(matches, test.state.clone())
					}
					if !known || !b {
						remaining = append(remaining, evaluation{state: test.state, values: path.values})
					}
				}
			}
			pending = a.limitEvaluations(fn, label, remaining)
		}
		result = append(result, a.statements(fn, clause.Body, matches, depth, handler)...)
	}
	for _, path := range pending {
		result = append(result, a.statements(fn, defaultBody, []flow{path.state}, depth, handler)...)
	}
	for i := range result {
		if result[i].branch == token.BREAK {
			result[i].ended = false
			result[i].branch = token.ILLEGAL
		}
	}
	return result
}

// 将声明和赋值关联到标准库 types 对象，不按变量名猜测身份。
// Bind assignments to types objects instead of variable-name guesses.
func (a *analyzer) assign(fn Function, lhs ast.Expr, value Value, state *flow) {
	if id, ok := lhs.(*ast.Ident); ok {
		obj := fn.Package.Info.ObjectOf(id)
		if obj != nil {
			state.values[obj] = coerceValue(value, obj.Type())
		}
		return
	}
	if star, ok := lhs.(*ast.StarExpr); ok {
		if id, ok := star.X.(*ast.Ident); ok {
			pointer := state.values[fn.Package.Info.ObjectOf(id)]
			if pointer.address != nil {
				state.values[pointer.address] = coerceValue(value, pointer.address.Type())
				return
			}
		}
		a.unknown(state, a.project.Source(lhs.Pos()), "指针写入的目标身份未解决")
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
