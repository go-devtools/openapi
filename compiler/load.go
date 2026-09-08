// Expose static loading and frontend extensions separately from runtime packages.
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
type Package struct {
	// Go type sizes for the actual loaded target; this view is read-only.
	Sizes       types.Sizes
	Path        string
	Name        string
	Types       *types.Package
	Info        *types.Info
	Files       []*ast.File
	SourceFiles []string
}

// Store a composable project view that supports concurrent reads after loading.
type Project struct {
	// Compile-only substituted field origins preserve frozen declaration metadata.
	substitutionOrigins map[types.Object]types.Object
	// Capture is private to Compile; public Schema calls never mutate it.
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
type Function struct {
	// Helper invocations retain concrete type arguments without changing the loaded declaration.
	substitution *typeSubstitution
	Object       *types.Func
	Signature    *types.Signature
	Declaration  *ast.FuncDecl
	Package      *Package
	Symbol       string
	Source       openapi.Source
}

// Load actual build-selected source and types without changing module files.
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
			// Anonymous response structs inside helpers retain their field declarations as well.
			if d.Body != nil {
				ast.Inspect(d.Body, func(node ast.Node) bool {
					if structure, ok := node.(*ast.StructType); ok {
						for _, field := range structure.Fields.List {
							for _, name := range field.Names {
								object := pkg.Info.Defs[name]
								p.metadataSymbols[p.metadataObject(object)] = symbol + "." + name.Name
								attach(object, field.Doc, field.Comment)
							}
						}
					}
					return true
				})
			}
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
								if direct[field] {
									symbol += "." + name.Name
								}
								p.metadataSymbols[p.metadataObject(object)] = symbol
								attach(object, field.Doc, field.Comment)
							}
						}
						return true
					})
				case *ast.ValueSpec:
					groups := []*ast.CommentGroup{s.Doc, s.Comment}
					// Read standalone constant comments from GenDecl; group headings do not describe individual enum values.
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
func (p *Project) Functions() []Function {
	out := make([]Function, 0, len(p.functions))
	for _, f := range p.functions {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}
