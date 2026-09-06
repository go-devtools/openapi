package compiler

import (
	"fmt"
	"go/scanner"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Resolve a type relative to one loaded source package without loading additional packages or mutating scopes.
// 相对于已加载的源码包解析类型，不额外加载包或修改作用域。
// Fully qualified package paths can occur inside pointers, collections, and generic arguments.
// 完整限定的包路径可出现在指针、集合及泛型实参中。
func (p *Project) TypeIn(packagePath, expression string) (types.Type, error) {
	if len(expression) == 0 || len(expression) > 8192 {
		return nil, fmt.Errorf("openapi.type.expression: a type expression must contain 1 to 8192 bytes")
	}
	var owner *types.Package
	packages := map[string]*types.Package{}
	var visit func(*types.Package)
	visit = func(pkg *types.Package) {
		if pkg == nil || packages[pkg.Path()] != nil {
			return
		}
		packages[pkg.Path()] = pkg
		for _, dependency := range pkg.Imports() {
			visit(dependency)
		}
	}
	for _, pkg := range p.Packages {
		visit(pkg.Types)
		if pkg.Path == packagePath {
			owner = pkg.Types
		}
	}
	if owner == nil {
		return nil, fmt.Errorf("openapi.type.package: package %s is not a loaded source root", packagePath)
	}
	scope := types.NewPackage(owner.Path(), owner.Name())
	for _, name := range owner.Scope().Names() {
		switch object := owner.Scope().Lookup(name).(type) {
		case *types.TypeName:
			scope.Scope().Insert(types.NewTypeName(token.NoPos, scope, name, object.Type()))
		case *types.Const:
			scope.Scope().Insert(types.NewConst(token.NoPos, scope, name, object.Type(), object.Val()))
		}
	}
	paths := sortedKeys(packages)
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) > len(paths[j])
		}
		return paths[i] < paths[j]
	})
	// Quoted literals and comments are data, even when they contain a package-looking substring.
	// 引号中的字面量和注释均视为数据，即使包含类似包路径的子串。
	protected := map[int]int{}
	fset := token.NewFileSet()
	file := fset.AddFile("expression", -1, len(expression))
	var scan scanner.Scanner
	scan.Init(file, []byte(expression), func(token.Position, string) {}, scanner.ScanComments)
	previous := -1
	for {
		pos, kind, _ := scan.Scan()
		// Scanner literals may normalize CR bytes; token positions retain exact source extents.
		// 扫描器可能规范化字面量中的 CR 字节；token 位置仍保留准确的源码范围。
		if previous >= 0 {
			protected[previous] = file.Offset(pos) - previous
			previous = -1
		}
		if kind == token.EOF {
			break
		}
		if kind == token.STRING || kind == token.CHAR || kind == token.COMMENT {
			previous = file.Offset(pos)
		}
	}
	var rewritten strings.Builder
	count := 0
	for offset := 0; offset < len(expression); {
		if size := protected[offset]; size > 0 {
			rewritten.WriteString(expression[offset : offset+size])
			offset += size
			continue
		}
		matched := false
		if offset == 0 || !typePathCharacter(expression[:offset]) {
			for _, path := range paths {
				prefix := path + "."
				if !strings.HasPrefix(expression[offset:], prefix) {
					continue
				}
				start := offset + len(prefix)
				end := start
				for end < len(expression) {
					r, size := utf8.DecodeRuneInString(expression[end:])
					if !unicode.IsLetter(r) && r != '_' && !(end > start && unicode.IsDigit(r)) {
						break
					}
					end += size
				}
				name := expression[start:end]
				object := packages[path].Scope().Lookup(name)
				if object == nil || path != owner.Path() && !object.Exported() {
					return nil, fmt.Errorf("openapi.type.unresolved: %s.%s is not an accessible loaded type or constant", path, name)
				}
				alias := fmt.Sprintf("_openapi_type_%d", count)
				for scope.Scope().Lookup(alias) != nil {
					alias += "_"
				}
				switch object := object.(type) {
				case *types.TypeName:
					scope.Scope().Insert(types.NewTypeName(token.NoPos, scope, alias, object.Type()))
				case *types.Const:
					scope.Scope().Insert(types.NewConst(token.NoPos, scope, alias, object.Type(), object.Val()))
				default:
					return nil, fmt.Errorf("openapi.type.unresolved: %s.%s is not a type or constant", path, name)
				}
				rewritten.WriteString(alias)
				offset = end
				count++
				matched = true
				break
			}
		}
		if !matched {
			rewritten.WriteByte(expression[offset])
			offset++
		}
	}
	value, err := types.Eval(p.Fset, scope, token.NoPos, rewritten.String())
	if err != nil || !value.IsType() || !instantiatedType(value.Type) {
		return nil, fmt.Errorf("openapi.type.unresolved: cannot resolve %s as a complete type in %s", expression, packagePath)
	}
	return value.Type, nil
}

// Prevent matching only the suffix of a longer identifier or import path.
// 避免只匹配较长标识符或导入路径的后缀。
func typePathCharacter(prefix string) bool {
	r, _ := utf8.DecodeLastRuneInString(prefix)
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_./-", r)
}

// Reject uninstantiated generic definitions and type parameters before schema projection.
// 在 Schema 投影前拒绝未实例化的泛型定义和类型参数。
func instantiatedType(t types.Type) bool {
	switch t := t.(type) {
	case nil, *types.TypeParam:
		return false
	case *types.Alias:
		if t.TypeParams().Len() > t.TypeArgs().Len() {
			return false
		}
		return instantiatedType(types.Unalias(t))
	case *types.Named:
		if t.TypeParams().Len() > t.TypeArgs().Len() {
			return false
		}
		for i := 0; i < t.TypeArgs().Len(); i++ {
			if !instantiatedType(t.TypeArgs().At(i)) {
				return false
			}
		}
	case *types.Pointer:
		return instantiatedType(t.Elem())
	case *types.Slice:
		return instantiatedType(t.Elem())
	case *types.Array:
		return instantiatedType(t.Elem())
	case *types.Map:
		return instantiatedType(t.Key()) && instantiatedType(t.Elem())
	}
	return true
}
