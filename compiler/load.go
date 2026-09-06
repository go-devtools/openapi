// Expose static loading and frontend extensions separately from runtime packages.
// 公开静态加载、源码视图与前端扩展能力；运行时包不导入本包。
package compiler

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/internal/comment"
	"golang.org/x/tools/go/packages"
)

// Configure read-only package loading without running generation scripts.
// 配置只读包加载和资源预算，不执行项目生成脚本。
type LoadOptions struct {
	Dir            string
	Patterns       []string
	BuildFlags     []string
	Env            []string
	Overlay        map[string][]byte
	MaxPackages    int
	MaxSourceBytes int64
}

// Expose standard-library types and AST as read-only views.
// 暴露标准库类型和 AST；调用方须将这些视图视为只读。
type Package struct {
	// Go type sizes for the actual loaded target; this view is read-only.
	// 实际加载目标的 Go 类型尺寸，视图只读。
	Sizes       types.Sizes
	Path        string
	Name        string
	Types       *types.Package
	Info        *types.Info
	Files       []*ast.File
	SourceFiles []string
}

// Store a composable project view that supports concurrent reads after loading.
// 保存可组合的静态项目视图，加载后可并发读取。
type Project struct {
	// Capture is private to Compile; public Schema calls never mutate it.
	// 捕获状态仅供 Compile 私有实例使用，公开 Schema 调用不修改它。
	explanations    *explanationCapture
	dependencies    []Package
	constants       []*types.Const
	commentErrors   map[types.Object]openapi.Diagnostic
	metadataSymbols map[types.Object]string
	sourceNames     map[string]string
	Dir             string
	Fset            *token.FileSet
	Packages        []Package
	comments        map[types.Object]comment.Document
	functions       map[*types.Func]Function
	inputs          *buildInputs
	Diagnostics     []openapi.Diagnostic
}

// Describe functions without assuming context parameters or return conventions.
// 用标准库视图表示任何函数形态，不预设 context 参数或返回值模式。
type Function struct {
	Object      *types.Func
	Signature   *types.Signature
	Declaration *ast.FuncDecl
	Package     *Package
	Symbol      string
	Source      openapi.Source
}

// Load actual build-selected source and types without changing module files.
// 按实际构建条件加载源码与类型，默认禁止修改模块文件。
func Load(ctx context.Context, options LoadOptions) (*Project, error) {
	dir, err := filepath.Abs(options.Dir)
	if err != nil {
		return nil, err
	}
	patterns := options.Patterns
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	flags := append([]string{"-mod=readonly"}, options.BuildFlags...)
	cfg := &packages.Config{Context: ctx, Dir: dir, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedTypesSizes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule | packages.NeedEmbedFiles, Fset: token.NewFileSet(), BuildFlags: flags, Overlay: options.Overlay, Env: append(os.Environ(), options.Env...)}
	inputs, err := prepareBuildInputs(ctx, options, cfg)
	if err != nil {
		return nil, err
	}
	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	max := options.MaxPackages
	if max == 0 {
		max = 2048
	}
	p := &Project{metadataSymbols: map[types.Object]string{}, commentErrors: map[types.Object]openapi.Diagnostic{}, sourceNames: map[string]string{}, inputs: inputs, Dir: dir, Fset: cfg.Fset, comments: map[types.Object]comment.Document{}, functions: map[*types.Func]Function{}}
	count := 0
	roots := map[string]bool{}
	for _, pkg := range loaded {
		roots[pkg.PkgPath] = true
	}
	var dependencies []*packages.Package
	var loadErrors []string
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		count++
		if !roots[pkg.PkgPath] {
			dependencies = append(dependencies, pkg)
		}
		for _, e := range pkg.Errors {
			loadErrors = append(loadErrors, strings.ReplaceAll(e.Error(), dir+string(filepath.Separator), ""))
		}
	})
	if count > max {
		return nil, fmt.Errorf("openapi.load.budget: package count %d exceeds %d", count, max)
	}
	if len(loadErrors) > 0 {
		sort.Strings(loadErrors)
		return nil, fmt.Errorf("openapi.load.failed: %s", strings.Join(loadErrors, "\n"))
	}
	if err := inputs.collect(loaded, options, flags); err != nil {
		return nil, err
	}
	for _, pkg := range loaded {
		p.Packages = append(p.Packages, Package{Path: pkg.PkgPath, Name: pkg.Name, Types: pkg.Types, Info: pkg.TypesInfo, Sizes: pkg.TypesSizes, Files: pkg.Syntax, SourceFiles: pkg.CompiledGoFiles})
	}
	sort.Slice(p.Packages, func(i, j int) bool { return p.Packages[i].Path < p.Packages[j].Path })
	for _, pkg := range dependencies {
		p.dependencies = append(p.dependencies, Package{Path: pkg.PkgPath, Name: pkg.Name, Types: pkg.Types, Info: pkg.TypesInfo, Sizes: pkg.TypesSizes, Files: pkg.Syntax, SourceFiles: pkg.CompiledGoFiles})
		for _, path := range pkg.CompiledGoFiles {
			p.sourceNames[filepath.Clean(path)] = pkg.PkgPath + "/" + filepath.Base(path)
		}
	}
	sort.Slice(p.dependencies, func(i, j int) bool { return p.dependencies[i].Path < p.dependencies[j].Path })
	for i := range p.dependencies {
		pkg := &p.dependencies[i]
		for _, file := range pkg.Files {
			p.indexFile(pkg, file, false)
		}
	}
	for i := range p.Packages {
		pkg := &p.Packages[i]
		for _, file := range pkg.Files {
			p.indexFile(pkg, file, true)
		}
	}
	for _, group := range [][]Package{p.Packages, p.dependencies} {
		for _, pkg := range group {
			for _, name := range pkg.Types.Scope().Names() {
				if value, ok := pkg.Types.Scope().Lookup(name).(*types.Const); ok {
					p.constants = append(p.constants, value)
				}
			}
		}
	}
	sort.Slice(p.constants, func(i, j int) bool {
		a, b := p.constants[i], p.constants[j]
		return a.Pkg().Path()+"."+a.Name() < b.Pkg().Path()+"."+b.Name()
	})
	if len(p.Diagnostics) > 0 {
		return nil, openapi.Report{Diagnostics: p.Diagnostics}
	}
	return p, nil
}

// Index semantic comments for functions, types, fields, and constants.
// 为函数、类型、字段及常量建立共享语义注释索引。
func (p *Project) indexFile(pkg *Package, file *ast.File, root bool) {
	attach := func(object types.Object, groups ...*ast.CommentGroup) { p.attachComment(object, root, groups...) }

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !root {
				continue
			}
			obj, _ := pkg.Info.Defs[d.Name].(*types.Func)
			if obj == nil {
				continue
			}
			attach(obj, d.Doc)
			sig, _ := obj.Type().(*types.Signature)
			symbol := pkg.Path + "." + obj.Name()
			if sig.Recv() != nil {
				symbol = types.TypeString(sig.Recv().Type(), func(p *types.Package) string { return p.Path() }) + "." + obj.Name()
			}
			p.functions[obj] = Function{Object: obj, Signature: sig, Declaration: d, Package: pkg, Symbol: symbol, Source: p.Source(d.Pos())}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					groups := []*ast.CommentGroup{s.Doc, s.Comment}
					if s.Doc == nil {
						groups = append([]*ast.CommentGroup{d.Doc}, groups...)
					}
					attach(pkg.Info.Defs[s.Name], groups...)
					direct := map[*ast.Field]bool{}
					if structure, ok := s.Type.(*ast.StructType); ok {
						for _, field := range structure.Fields.List {
							direct[field] = true
						}
					}
					ast.Inspect(s.Type, func(n ast.Node) bool {
						if field, ok := n.(*ast.Field); ok {
							for _, name := range field.Names {
								object := pkg.Info.Defs[name]
								symbol := pkg.Path + "." + s.Name.Name
								// Nested anonymous fields use their real enclosing named declaration.
								// 嵌套匿名字段使用其实际所属的命名声明。
								if direct[field] {
									symbol += "." + name.Name
								}
								p.metadataSymbols[metadataObject(object)] = symbol
								attach(object, field.Doc, field.Comment)
							}
						}
						return true
					})
				case *ast.ValueSpec:
					groups := []*ast.CommentGroup{s.Doc, s.Comment}
					// Read standalone constant comments from GenDecl; group headings do not describe individual enum values.
					// 单独声明的常量注释位于 GenDecl；分组标题不冒充单个枚举的含义。
					if s.Doc == nil && !d.Lparen.IsValid() {
						groups = append([]*ast.CommentGroup{d.Doc}, groups...)
					}
					for _, name := range s.Names {
						attach(pkg.Info.Defs[name], groups...)
					}
				}
			}
		}
	}
}

// Convert positions into project-relative source paths.
// 将源码位置转换为项目相对路径。
func (p *Project) Source(pos token.Pos) openapi.Source {
	if !pos.IsValid() {
		return openapi.Source{}
	}
	v := p.Fset.PositionFor(pos, false)
	if logical, ok := p.sourceNames[filepath.Clean(v.Filename)]; ok {
		return openapi.Source{File: logical, Line: v.Line, Column: v.Column}
	}
	file, err := filepath.Rel(p.Dir, v.Filename)
	if err != nil {
		file = filepath.Base(v.Filename)
	}
	return openapi.Source{File: filepath.ToSlash(file), Line: v.Line, Column: v.Column}
}

// Resolve real types and generic instances from already loaded packages.
// 解析当前包或已加载模块限定的真实类型及泛型实例。
func (p *Project) Type(name string) (types.Type, error) {
	var result types.Type
	for _, pkg := range p.Packages {
		resolved, err := p.TypeIn(pkg.Path, name)
		if err != nil {
			continue
		}
		if result != nil && !types.Identical(result, resolved) {
			return nil, fmt.Errorf("openapi.type.ambiguous: %s resolves to different types; use a qualified expression or TypeIn", name)
		}
		result = resolved
	}
	if result != nil {
		return result, nil
	}
	return nil, fmt.Errorf("openapi.type.unresolved: cannot resolve %s in the loaded project", name)
}

// Return function views sorted by stable source symbols.
// 返回按稳定符号排序的只读函数视图。
func (p *Project) Functions() []Function {
	out := make([]Function, 0, len(p.functions))
	for _, f := range p.functions {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}
