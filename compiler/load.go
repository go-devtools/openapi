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
	Dir         string
	Fset        *token.FileSet
	Packages    []Package
	comments    map[types.Object]comment.Document
	functions   map[*types.Func]Function
	inputs      *buildInputs
	Diagnostics []openapi.Diagnostic
}

// Describe functions without assuming context parameters or return conventions.
type Function struct {
	Object      *types.Func
	Signature   *types.Signature
	Declaration *ast.FuncDecl
	Package     *Package
	Symbol      string
	Source      openapi.Source
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
	p := &Project{inputs: inputs, Dir: dir, Fset: cfg.Fset, comments: map[types.Object]comment.Document{}, functions: map[*types.Func]Function{}}
	count := 0
	var loadErrors []string
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		count++
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
	for i := range p.Packages {
		pkg := &p.Packages[i]
		for _, file := range pkg.Files {
			p.indexFile(pkg, file)
		}
	}
	if len(p.Diagnostics) > 0 {
		return nil, openapi.Report{Diagnostics: p.Diagnostics}
	}
	return p, nil
}

// Index semantic comments for functions, types, fields, and constants.
func (p *Project) indexFile(pkg *Package, file *ast.File) {
	attach := func(obj types.Object, groups ...*ast.CommentGroup) {
		if obj == nil {
			return
		}
		var parts []string
		for _, g := range groups {
			if g != nil {
				parts = append(parts, g.Text())
			}
		}
		d, err := comment.Parse(strings.Join(parts, "\n"))
		if err != nil {
			p.Diagnostics = append(p.Diagnostics, openapi.Diagnostic{Code: "openapi.comment.invalid", Severity: openapi.Error, Message: err.Error(), Source: p.Source(obj.Pos()), Fix: "Fix the unified @openapi directive"})
			return
		}
		p.comments[obj] = d
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
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
					ast.Inspect(s.Type, func(n ast.Node) bool {
						if field, ok := n.(*ast.Field); ok {
							for _, name := range field.Names {
								attach(pkg.Info.Defs[name], field.Doc, field.Comment)
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
	v := p.Fset.Position(pos)
	file, err := filepath.Rel(p.Dir, v.Filename)
	if err != nil {
		file = filepath.Base(v.Filename)
	}
	return openapi.Source{File: filepath.ToSlash(file), Line: v.Line, Column: v.Column}
}

// Resolve real types and generic instances from already loaded packages.
func (p *Project) Type(name string) (types.Type, error) {
	for _, pkg := range p.Packages {
		expr := name
		if strings.HasPrefix(name, pkg.Path+".") {
			expr = strings.TrimPrefix(name, pkg.Path+".")
		} else if strings.Contains(name, "/") {
			continue
		}
		value, err := types.Eval(p.Fset, pkg.Types, token.NoPos, expr)
		if err == nil && value.IsType() {
			return value.Type, nil
		}
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
