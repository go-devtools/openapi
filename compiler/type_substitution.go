package compiler

import (
	"fmt"
	"go/ast"
	"go/types"
)

// Keep concrete generic arguments and type identities local to one helper invocation.
type typeSubstitution struct {
	project   *Project
	arguments map[*types.TypeParam]types.Type
	cache     map[types.Type]types.Type
	context   *types.Context
	err       error
}

// Read a source type through the current helper's concrete generic arguments.
func (f Function) concrete(t types.Type) types.Type {
	if f.substitution == nil {
		return t
	}
	return f.substitution.apply(t)
}

// Substitute one finite type graph without changing loaded types or running user code.
func (s *typeSubstitution) apply(t types.Type) types.Type {
	if t == nil || s.err != nil {
		return t
	}
	if actual, ok := s.cache[t]; ok {
		return actual
	}
	if len(s.cache) >= 4096 {
		s.err = fmt.Errorf("openapi.analysis.types: generic helper substitution exceeds the 4096-type budget")
		return t
	}
	s.cache[t] = t
	result := t
	switch x := t.(type) {
	case *types.TypeParam:
		if actual := s.arguments[x]; actual != nil {
			result = actual
		}
	case *types.Pointer:
		if elem := s.apply(x.Elem()); elem != x.Elem() {
			result = types.NewPointer(elem)
		}
	case *types.Slice:
		if elem := s.apply(x.Elem()); elem != x.Elem() {
			result = types.NewSlice(elem)
		}
	case *types.Array:
		if elem := s.apply(x.Elem()); elem != x.Elem() {
			result = types.NewArray(elem, x.Len())
		}
	case *types.Map:
		key, elem := s.apply(x.Key()), s.apply(x.Elem())
		if key != x.Key() || elem != x.Elem() {
			result = types.NewMap(key, elem)
		}
	case *types.Chan:
		if elem := s.apply(x.Elem()); elem != x.Elem() {
			result = types.NewChan(x.Dir(), elem)
		}
	case *types.Named:
		result = s.instance(x, x.Origin(), x.TypeArgs())
	case *types.Alias:
		result = s.instance(x, x.Origin(), x.TypeArgs())
	case *types.Struct:
		fields, tags := make([]*types.Var, x.NumFields()), make([]string, x.NumFields())
		changed := false
		for i := range fields {
			fields[i], tags[i] = s.variable(x.Field(i)), x.Tag(i)
			changed = changed || fields[i] != x.Field(i)
		}
		if changed {
			result = types.NewStruct(fields, tags)
		}
	case *types.Tuple:
		result = s.tuple(x)
	case *types.Signature:
		// A declared generic signature owns its own parameters; instantiated signatures contain only free outer parameters.
		if x.TypeParams().Len() != 0 || x.RecvTypeParams().Len() != 0 {
			break
		}
		receiver, parameters, results := s.variable(x.Recv()), s.tuple(x.Params()), s.tuple(x.Results())
		if receiver != x.Recv() || parameters != x.Params() || results != x.Results() {
			result = types.NewSignatureType(receiver, nil, nil, parameters, results, x.Variadic())
		}
	case *types.Interface:
		methods, embeds := make([]*types.Func, x.NumExplicitMethods()), make([]types.Type, x.NumEmbeddeds())
		changed := false
		for i := range methods {
			original := x.ExplicitMethod(i)
			signature := s.apply(original.Type())
			methods[i] = original
			if signature != original.Type() {
				methods[i] = types.NewFunc(original.Pos(), original.Pkg(), original.Name(), signature.(*types.Signature))
				changed = true
			}
		}
		for i := range embeds {
			embeds[i] = s.apply(x.EmbeddedType(i))
			changed = changed || embeds[i] != x.EmbeddedType(i)
		}
		if changed {
			result = types.NewInterfaceType(methods, embeds).Complete()
		}
	}
	s.cache[t] = result
	return result
}

// Instantiate named and alias types with substituted arguments while preserving Go declaration origins.
func (s *typeSubstitution) instance(original, origin types.Type, arguments *types.TypeList) types.Type {
	if arguments.Len() == 0 {
		return original
	}
	actual := make([]types.Type, arguments.Len())
	changed := false
	for i := range actual {
		actual[i] = s.apply(arguments.At(i))
		changed = changed || actual[i] != arguments.At(i)
	}
	if !changed {
		return original
	}
	result, err := types.Instantiate(s.context, origin, actual, false)
	if err != nil {
		s.err = fmt.Errorf("openapi.analysis.types: cannot instantiate helper type: %w", err)
		return original
	}
	return result
}

// Retain original field metadata when substituting anonymous composite members.
func (s *typeSubstitution) variable(original *types.Var) *types.Var {
	if original == nil {
		return nil
	}
	actual := s.apply(original.Type())
	if actual == original.Type() {
		return original
	}
	result := types.NewVar(original.Pos(), original.Pkg(), original.Name(), actual)
	if original.IsField() {
		result = types.NewField(original.Pos(), original.Pkg(), original.Name(), actual, original.Embedded())
	}
	if s.project.substitutionOrigins == nil {
		s.project.substitutionOrigins = map[types.Object]types.Object{}
	}
	s.project.substitutionOrigins[result] = original
	return result
}

// Preserve tuple order and unchanged Go variable identities.
func (s *typeSubstitution) tuple(original *types.Tuple) *types.Tuple {
	if original == nil {
		return nil
	}
	variables := make([]*types.Var, original.Len())
	changed := false
	for i := range variables {
		variables[i] = s.variable(original.At(i))
		changed = changed || variables[i] != original.At(i)
	}
	if !changed {
		return original
	}
	return types.NewTuple(variables...)
}

// Bind explicit, inferred, captured, and receiver type arguments before evaluating a helper body.
func (a *analyzer) instantiateFunction(call CallContext, helper Function) (Function, error) {
	arguments := map[*types.TypeParam]types.Type{}
	if helper.substitution != nil {
		for parameter, actual := range helper.substitution.arguments {
			arguments[parameter] = actual
		}
	}
	parameters := helper.Signature.TypeParams()
	if parameters.Len() > 0 {
		if call.callee == nil || len(call.callee.typeArguments) != parameters.Len() {
			return helper, fmt.Errorf("openapi.analysis.types: generic helper arguments are unresolved")
		}
		for i := 0; i < parameters.Len(); i++ {
			arguments[parameters.At(i)] = call.callee.typeArguments[i]
		}
	}
	receiverParameters := helper.Signature.RecvTypeParams()
	if receiverParameters.Len() > 0 {
		if call.Receiver.Type == nil {
			return helper, fmt.Errorf("openapi.analysis.types: generic receiver type is unresolved")
		}
		receiver := types.Unalias(call.Receiver.Type)
		if pointer, ok := receiver.(*types.Pointer); ok {
			receiver = types.Unalias(pointer.Elem())
		}
		named, ok := receiver.(*types.Named)
		if !ok || named.TypeArgs().Len() != receiverParameters.Len() {
			return helper, fmt.Errorf("openapi.analysis.types: generic receiver arguments are unresolved")
		}
		for i := 0; i < receiverParameters.Len(); i++ {
			arguments[receiverParameters.At(i)] = named.TypeArgs().At(i)
		}
	}
	if len(arguments) > 0 {
		helper.substitution = &typeSubstitution{project: a.project, arguments: arguments, cache: map[types.Type]types.Type{}, context: types.NewContext()}
	}
	return helper, nil
}

// Read only go/types-confirmed function instantiations, including inferred and nested type arguments.
func (f Function) instanceArguments(expression ast.Expr) []types.Type {
	var identifier *ast.Ident
	switch x := expression.(type) {
	case *ast.Ident:
		identifier = x
	case *ast.SelectorExpr:
		identifier = x.Sel
	case *ast.ParenExpr:
		return f.instanceArguments(x.X)
	default:
		return nil
	}
	instance, ok := f.Package.Info.Instances[identifier]
	if !ok {
		return nil
	}
	if _, ok := f.Package.Info.ObjectOf(identifier).(*types.Func); !ok {
		return nil
	}
	arguments := make([]types.Type, instance.TypeArgs.Len())
	for i := range arguments {
		arguments[i] = f.concrete(instance.TypeArgs.At(i))
	}
	return arguments
}
