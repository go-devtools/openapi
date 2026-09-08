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

// Track values, effects, and termination for one reachable path.
type flow struct {
	when            openapi.RequestCondition
	hasCommit       bool
	headers         map[string]HeaderValue
	observedHeaders map[string]HeaderValue
	values          map[uint64]Value
	bindings        map[types.Object]uint64
	effects         []Effect
	diagnostics     []openapi.Diagnostic
	ended           bool
	writes          int
	bodyKind        EffectKind
	bodyMedia       string
	pending         *Effect
	returned        []Value
	committed       string
	branch          token.Token
}

// Keep a bounded analyzer with independent state for each handler.
type analyzer struct {
	// Summaries are private to one candidate and never enter the runtime Bundle.
	summaries       *helperSummaryCache
	summaryCaptures []*helperSummaryCapture
	ctx             context.Context
	project         *Project
	options         Options
	frontend        Frontend
	calls           int
	nextCell        uint64
	diagnostics     []openapi.Diagnostic
}

// Copy path state so branches cannot mutate each other.
func (s flow) clone() flow {
	out := s
	out.headers = copyHeaders(s.headers)
	out.observedHeaders = copyHeaders(s.observedHeaders)
	out.values = map[uint64]Value{}
	out.bindings = copyBindings(s.bindings)
	for k, v := range s.values {
		out.values[k] = v
	}
	out.effects = append([]Effect(nil), s.effects...)
	out.diagnostics = append([]openapi.Diagnostic(nil), s.diagnostics...)
	return out
}

// Record a diagnostic when reliable analysis cannot continue.
func (a *analyzer) unknown(s *flow, source openapi.Source, message string) {
	s.diagnostics = append(s.diagnostics, openapi.Diagnostic{Code: "openapi.analysis.unresolved", Severity: openapi.Error, Message: message, Fix: "Provide a centralized frontend rule or simplify control flow to fit the analysis budget", Source: source})
}

// Propagate values in Go execution order and stop after return.
func (a *analyzer) statements(fn Function, statements []ast.Stmt, paths []flow, depth int, handler bool) []flow {
	if depth > a.options.MaxDepth {
		for i := range paths {
			a.unknown(&paths[i], fn.Source, "cross-function depth exceeds the budget")
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
				a.unknown(&paths[i], a.project.Source(statement.Pos()), "control-flow paths exceed the budget")
			}
			break
		}
	}
	return paths
}

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
				value := Value{Type: fn.concrete(fn.Package.Info.TypeOf(lhs)), Unknown: true}
				if i < len(e.values) {
					value = e.values[i]
				}
				if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
					a.unknown(&e.state, source, "compound assignment requires explicit operator propagation")
					value = Value{Type: fn.concrete(fn.Package.Info.TypeOf(lhs)), Unknown: true}
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
								value := zeroValue(fn.concrete(obj.Type()))
								if i < len(e.values) {
									value = e.values[i]
								}
								a.bind(&e.state, obj, coerceValue(value, fn.concrete(obj.Type())))
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
					e.values = append(e.values, e.state.read(obj))
				}
			}
			for i := range e.values {
				if i < fn.Signature.Results().Len() {
					e.values[i] = coerceValue(e.values[i], fn.concrete(fn.Signature.Results().At(i).Type()))
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
		a.unknown(&state, source, "effects inside a loop require a bounded summary or centralized adaptation")
	case *ast.GoStmt, *ast.DeferStmt:
		a.unknown(&state, source, "response effects of asynchronous or deferred calls require centralized adaptation")
	case *ast.IncDecStmt:
		var result []flow
		for _, path := range a.evaluate(fn, s.X, state, depth) {
			value := incrementValue(scalar(path), fn.concrete(fn.Package.Info.TypeOf(s.X)), fn.Package.Sizes, s.Tok)
			a.assign(fn, s.X, value, &path.state)
			result = append(result, path.state)
		}
		return result
	case *ast.BranchStmt:
		if s.Tok != token.BREAK || s.Label != nil {
			a.unknown(&state, source, "unsupported control-flow jump")
		}
		state.ended = true
		state.branch = s.Tok
	case *ast.EmptyStmt:
	default:
		a.unknown(&state, source, fmt.Sprintf("unresolved statement %T", statement))
	}
	return []flow{state}
}

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

// Bind assignments to types objects instead of variable-name guesses.
func (a *analyzer) assign(fn Function, lhs ast.Expr, value Value, state *flow) {
	if id, ok := lhs.(*ast.Ident); ok {
		obj := fn.Package.Info.ObjectOf(id)
		if obj != nil {
			if obj.Pos() == lhs.Pos() || state.bindings[obj] == 0 {
				a.bind(state, obj, coerceValue(value, fn.concrete(obj.Type())))
			} else {
				state.values[state.bindings[obj]] = coerceValue(value, fn.concrete(obj.Type()))
			}
		}
		return
	}
	if star, ok := lhs.(*ast.StarExpr); ok {
		if id, ok := star.X.(*ast.Ident); ok {
			pointer := state.read(fn.Package.Info.ObjectOf(id))
			if pointer.address != 0 {
				state.values[pointer.address] = coerceValue(value, state.values[pointer.address].Type)
				return
			}
		}
		a.unknown(state, a.project.Source(lhs.Pos()), "pointer assignment target identity is unresolved")
		return
	}

	if field, ok := lhs.(*ast.SelectorExpr); ok {
		if id, ok := field.X.(*ast.Ident); ok {
			object := fn.Package.Info.ObjectOf(id)
			cell := state.bindings[object]
			owner := state.read(object)
			selection := fn.Package.Info.Selections[field]
			if owner.Type != nil {
				if _, pointer := owner.Type.Underlying().(*types.Pointer); pointer {
					cell = owner.address
					owner = state.values[cell]
				}
			}
			if cell != 0 && selection != nil && len(selection.Index()) == 1 {
				fields := map[string]Value{}
				for name, old := range owner.Fields {
					fields[name] = old
				}
				fields[field.Sel.Name] = coerceValue(value, fn.concrete(fn.Package.Info.TypeOf(lhs)))
				owner.Fields = fields
				state.values[cell] = owner
				return
			}
		}
		a.unknown(state, a.project.Source(lhs.Pos()), "field assignment target identity or nested path is unresolved")
		return
	}

	if index, ok := lhs.(*ast.IndexExpr); ok {
		if id, ok := index.X.(*ast.Ident); ok {
			obj := fn.Package.Info.ObjectOf(id)
			old := state.read(obj)
			key := fn.Package.Info.Types[index.Index].Value
			if key != nil && key.Kind() == constant.String {
				fields := map[string]Value{}
				for k, v := range old.Fields {
					fields[k] = v
				}
				fields[constant.StringVal(key)] = value
				old.Fields = fields
				state.values[state.bindings[obj]] = old
			} else {
				old.Unknown = true
				state.values[state.bindings[obj]] = old
			}
		}
	}
}

// Track write order; Abort does not imply a Go return.
func (a *analyzer) effects(state *flow, effects []Effect) {
	for _, e := range effects {
		if e.NonEmptyBody && e.Kind != RequestBody && e.Kind != RequestField {
			a.unknown(state, e.Source, "nonempty body proof requires a request-body or request-field effect")
		}
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
		case ResponseBody, ResponseItem:
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
			if state.writes > 1 && (e.Kind != ResponseItem || state.bodyKind != ResponseItem || state.bodyMedia != e.MediaType) {
				a.unknown(state, e.Source, "multiple bodies are written sequentially on one path and cannot be represented as response alternatives; only item-wise responses with the same media type can be written sequentially")
			}
			state.bodyKind, state.bodyMedia = e.Kind, e.MediaType
			state.effects = append(state.effects, e)
			state.pending = nil
		default:
			state.effects = append(state.effects, e)
		}
	}
}

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
