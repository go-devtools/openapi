package compiler

import (
	"bytes"
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/internal/comment"
)

// Keep physical directive offsets from the exact parsed bytes; AST comment text may normalize CR characters.
func (b *buildInputs) captureCommentPositions(file *ast.File, fset *token.FileSet, raw []byte) {
	if file == nil {
		return
	}
	positions := map[token.Pos][]token.Pos{}
	for _, group := range file.Comments {
		for _, c := range group.List {
			tokenFile := fset.File(c.Slash)
			if tokenFile == nil {
				continue
			}
			start := tokenFile.Offset(c.Slash)
			if start < 0 || start >= len(raw) {
				continue
			}
			text := raw[start:]
			if bytes.HasPrefix(text, []byte("//")) {
				if end := bytes.IndexByte(text, '\n'); end >= 0 {
					text = text[:end]
				}
			} else if end := bytes.Index(text, []byte("*/")); end >= 0 {
				text = text[:end+2]
			}
			offset := 0
			for _, line := range bytes.Split(text, []byte("\n")) {
				if directiveLine(string(line)) {
					index := bytes.Index(line, []byte("@openapi"))
					positions[c.Slash] = append(positions[c.Slash], tokenFile.Pos(start+offset+index))
				}
				offset += len(line) + 1
			}
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, values := range positions {
		b.commentPositions[key] = values
	}
}

// Recognize the same line prefix as the shared parser without interpreting prose as a directive.
func directiveLine(line string) bool {
	line = strings.TrimSpace(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
	line = strings.TrimSpace(strings.TrimPrefix(line, "/*"))
	return strings.HasPrefix(line, "@openapi")
}

// Return original positions in the same order as the supplied semantic comment groups.
func (p *Project) directivePositions(groups ...*ast.CommentGroup) []token.Pos {
	var positions []token.Pos
	if p.inputs == nil {
		return positions
	}
	for _, group := range groups {
		if group != nil {
			for _, c := range group.List {
				positions = append(positions, p.inputs.commentPositions[c.Slash]...)
			}
		}
	}
	return positions
}

// Associate instantiated generic fields with their declaration's comments and diagnostics.
func (p *Project) metadataObject(object types.Object) types.Object {
	for {
		if field, ok := object.(*types.Var); ok {
			object = field.Origin()
		}
		if original := p.substitutionOrigins[object]; original != nil {
			object = original
			continue
		}
		break
	}
	return object
}

// Freeze metadata errors during loading and surface dependency errors only when their actual objects are projected.
func (p *Project) attachComment(object types.Object, root bool, groups ...*ast.CommentGroup) {
	if object == nil {
		return
	}
	object = p.metadataObject(object)
	var parts []string
	for _, group := range groups {
		if group != nil {
			parts = append(parts, group.Text())
		}
	}
	text := strings.Join(parts, "\n")
	doc, err := comment.Parse(text)
	if err == nil {
		p.comments[object] = doc
		return
	}
	source := p.metadataSource(object)
	var issue *comment.Error
	if errors.As(err, &issue) {
		ordinal := -1
		for i, line := range strings.Split(text, "\n") {
			if i >= issue.Line {
				break
			}
			if directiveLine(line) {
				ordinal++
			}
		}
		positions := p.directivePositions(groups...)
		if ordinal >= 0 && ordinal < len(positions) {
			position := positions[ordinal] + token.Pos(issue.Column-1)
			located := p.Source(position)
			source.File, source.Line, source.Column = located.File, located.Line, located.Column
		}
	}
	diagnostic := openapi.Diagnostic{Code: "openapi.comment.invalid", Severity: openapi.Error, Message: err.Error(), Source: source, Fix: "Fix the unified @openapi directive on the referenced source declaration"}
	p.commentErrors[object] = diagnostic
	if root {
		p.Diagnostics = append(p.Diagnostics, diagnostic)
	}
}

// Read immutable metadata through original Go identities rather than requiring every DTO package to be a source root.
func (p *Project) metadata(object types.Object) (comment.Document, error) {
	object = p.metadataObject(object)
	if issue, ok := p.commentErrors[object]; ok {
		return comment.Document{}, openapi.Report{Diagnostics: []openapi.Diagnostic{issue}}
	}
	return p.comments[object], nil
}

// Preserve structured source diagnostics returned by shared projection instead of replacing them with call-site-only text.
func projectionDiagnostics(err error, source openapi.Source) []openapi.Diagnostic {
	var report openapi.Report
	if !errors.As(err, &report) {
		return nil
	}
	diagnostics := append([]openapi.Diagnostic(nil), report.Diagnostics...)
	for i := range diagnostics {
		diagnostics[i].Facts = append(append([]openapi.Source(nil), diagnostics[i].Facts...), source)
	}
	return diagnostics
}

// Locate the original source object; anonymous nested fields refer to their real enclosing named declaration.
func (p *Project) metadataSource(object types.Object) openapi.Source {
	object = p.metadataObject(object)
	if object == nil {
		return openapi.Source{Kind: "declared", Rule: "openapi.comment"}
	}
	source := p.Source(object.Pos())
	source.Kind, source.Rule = "declared", "openapi.comment"
	if function, ok := object.(*types.Func); ok {
		source.Symbol = function.FullName()
	} else if object.Pkg() != nil {
		source.Symbol = object.Pkg().Path() + "." + object.Name()
	}
	if symbol := p.metadataSymbols[object]; symbol != "" {
		source.Symbol = symbol
	}
	return source
}

// Preserve the shared annotation code while adding a structured declaration source for programmatic diagnostics.
func (p *Project) annotationIssue(err error, object types.Object) error {
	if err == nil {
		return nil
	}
	var existing openapi.Report
	if errors.As(err, &existing) {
		return err
	}
	code := "openapi.schema.annotation"
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		prefix, _, ok := strings.Cut(cause.Error(), ":")
		if ok && strings.HasPrefix(prefix, "openapi.") && !strings.ContainsAny(prefix, " \t\n") {
			code = prefix
			break
		}
	}
	message := strings.Replace(err.Error(), code+": ", "", 1)
	return openapi.Report{Diagnostics: []openapi.Diagnostic{{Code: code, Severity: openapi.Error, Message: message, Fix: "Keep source annotations consistent with the actual type and codec projection", Source: p.metadataSource(object)}}}
}
