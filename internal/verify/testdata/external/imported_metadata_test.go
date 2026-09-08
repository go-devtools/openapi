package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/compiler"
	"github.com/go-devtools/openapi/contracttest"
)

// Build a real imported DTO package while selecting only the application's package as a source root.
func metadataProject(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	dto := `package dto
// Closed application roles.
// @openapi enum
type Role string
const (
    // Administrator.
    Admin Role = "admin"
    // Editor.
    Editor Role = "editor"
)
// An open string type does not become an enum merely because constants exist.
type State string
const Ready State = "ready"
// An imported request with a client contract.
type Request struct {
    // Display name from the imported DTO.
    // @openapi required minLength=3 examples=["Alice"]
    Name string
    // Selected role.
    Role Role
    State State
}
// A generic envelope from the imported package.
type Envelope[T any] struct {
    // The actual payload from the imported envelope.
    // @openapi required
    Data T
}
// An unrelated imported type with malformed metadata.
type Unused struct {
    // @openapi examples=[
    Value string
}
// Imported functions must not become application operation candidates.
func LibraryHandler(value Request) Request { return value }
`
	files := map[string]string{
		"go.mod":     "module example.test/metadata\n\ngo 1.27.1\n",
		"dto/dto.go": dto,
		"app/app.go": `package app
import "example.test/metadata/dto"
// Keep the original parameter and return convention.
func Handle(value dto.Request) dto.Envelope[dto.Request] { return dto.Envelope[dto.Request]{Data:value} }
`,
	}
	for name, raw := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "app"), dto
}

// Imported fields, closed enums, and generic field origins must retain declarations without broadening operation discovery.
func TestImportedMetadata(t *testing.T) {
	dir, _ := metadataProject(t)
	front := compiler.Frontend{Name: "imported-metadata-v1", Match: func(f compiler.Function) bool { return f.Signature.Results().Len() == 1 },
		Entry: func(f compiler.Function) []compiler.Effect {
			return []compiler.Effect{{Kind: compiler.RequestBody, MediaType: "application/json", Payload: compiler.Value{Type: f.Signature.Params().At(0).Type()}, Source: f.Source}}
		},
		Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
			return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "200", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
		},
	}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{front}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Bundle.Index()) != 1 {
		t.Fatalf("imported functions entered candidate discovery: %+v", result.Bundle.Index())
	}
	document, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/metadata", OperationKey: "example.test/metadata/app.Handle"}}, openapi.Config{Title: "Imported DTO", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	raw := document.JSON()
	for _, text := range []string{"Display name from the imported DTO.", "The actual payload from the imported envelope.", "Administrator.", "Editor."} {
		if !strings.Contains(string(raw), text) {
			t.Errorf("lost imported metadata: %s", text)
		}
	}
	input, err := contracttest.Compile(raw, "/paths/~1metadata/post/requestBody/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := input.JSON([]byte(`{"Name":"Alice","Role":"admin","State":"custom"}`)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{"Name":"Al","Role":"admin"}`, `{"Role":"admin"}`, `{"Name":"Alice","Role":"unknown"}`} {
		if input.JSON([]byte(invalid)) == nil {
			t.Errorf("imported contract accepted invalid input: %s", invalid)
		}
	}
	output, err := contracttest.Compile(raw, "/paths/~1metadata/post/responses/200/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := output.JSON([]byte(`{"Data":{"Name":"Alice","Role":"editor","State":"custom"}}`)); err != nil {
		t.Fatal(err)
	}
	if output.JSON([]byte(`{"Data":{"Name":"A","Role":"editor","State":"custom"}}`)) == nil {
		t.Error("imported generic response lost its field constraint")
	}
}

// Validate only referenced imported metadata and keep the frozen loaded view independent of later filesystem writes.
func TestImportedMetadataSnapshotAndErrors(t *testing.T) {
	dir, dto := metadataProject(t)
	project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off", "GOPROXY=off"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Packages) != 1 || len(project.Functions()) != 1 {
		t.Fatal("dependency indexing changed the public source-root views")
	}
	typ, err := project.Type("example.test/metadata/dto.Request")
	if err != nil {
		t.Fatal(err)
	}

	// A loaded Project must not reread source to obtain dependency comments.
	path := filepath.Join(dir, "../dto/dto.go")
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(dto, "minLength=3", "minLength=20")), 0600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			projection, err := project.Schema(compiler.ProjectionRequest{Type: typ, Direction: compiler.Input})
			if err != nil {
				failures <- err
				return
			}
			raw, err := projection.Standalone()
			if err != nil {
				failures <- err
				return
			}
			check, err := contracttest.Compile(raw, "", contracttest.Options{})
			if err != nil {
				failures <- err
				return
			}
			if err := check.JSON([]byte(`{"Name":"Alice","Role":"admin"}`)); err != nil {
				failures <- err
			}
			if check.JSON([]byte(`{"Name":"Al","Role":"admin"}`)) == nil {
				failures <- &metadataFailure{}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	unused, err := project.Type("example.test/metadata/dto.Unused")
	if err != nil {
		t.Fatal(err)
	}
	_, err = project.Schema(compiler.ProjectionRequest{Type: unused, Direction: compiler.Input})
	if err == nil || !strings.Contains(err.Error(), "openapi.comment.invalid") {
		t.Fatalf("referenced malformed imported metadata was silently ignored: %v", err)
	}
}

// Report a schema assertion from a concurrent read without calling Fatal inside a worker.
type metadataFailure struct{}

// Explain the lost imported constraint.
func (*metadataFailure) Error() string {
	return "imported metadata was not retained in the immutable loaded view"
}

// Preserve actual directive locations in CRLF blocks and report coordinates for the loaded overlay rather than a later file.
func TestCommentSourcePositions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.go")
	base := "package app\n// Ordinary comment.\nfunc H() {}\n"
	source := "package app\r\n/* A multiline operation comment.\r\n\r\n\t@openapi response status=204\r\n*/\r\nfunc H() {}\r\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/positions\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Overlay: map[string][]byte{path: []byte(source)}, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{{Name: "source-position-v1", Match: func(f compiler.Function) bool { return f.Object.Name() == "H" }}}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "DELETE", Path: "/source", OperationKey: "example.test/positions.H"}}, openapi.Config{Title: "Source", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range document.Report().Facts {
		if fact.Kind == "declared" {
			found = true
			if fact.File != "app.go" || fact.Line != 4 || fact.Column != 2 {
				raw, _ := json.Marshal(fact)
				t.Errorf("wrong overlay directive position: %s", raw)
			}
		}
	}
	if !found {
		t.Fatal("missing declaration provenance")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != base {
		t.Fatal("source or overlay was written to the business file", err)
	}
}

// Keep a referenced dependency error at its physical source while retaining the calling effect or declaration as evidence.
func TestImportedMetadataDiagnosticSources(t *testing.T) {
	dir, dto := metadataProject(t)
	path := filepath.Join(dir, "app.go")
	source := `package app
import "example.test/metadata/dto"
func Good(value dto.Request) dto.Request { return value }
func Bad(value dto.Unused) dto.Unused { return value }
// @openapi response status=200 mediaType="application/json" type="example.test/metadata/dto.Unused"
func Declared() {}
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	front := compiler.Frontend{Name: "metadata-errors-v1", Match: func(f compiler.Function) bool { return true }, Entry: func(f compiler.Function) []compiler.Effect {
		if f.Signature.Params().Len() == 0 {
			return nil
		}
		return []compiler.Effect{{Kind: compiler.RequestBody, MediaType: "application/json", Payload: compiler.Value{Type: f.Signature.Params().At(0).Type()}, Source: f.Source}}
	}, Return: func(c compiler.ReturnContext) ([]compiler.Effect, error) {
		if len(c.Values) == 0 {
			return nil, nil
		}
		return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "200", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
	}}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{front}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/good", OperationKey: "example.test/metadata/app.Good"}}, openapi.Config{Title: "Good", Version: "1"}); err != nil {
		t.Fatal(err)
	}
	expectedLine := strings.Count(dto[:strings.Index(dto, "    // @openapi examples=[")], "\n") + 1
	for _, name := range []string{"Bad", "Declared"} {
		t.Run(name, func(t *testing.T) {
			_, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/bad", OperationKey: openapi.OperationKey("example.test/metadata/app." + name)}}, openapi.Config{Title: "Bad", Version: "1"})
			var report openapi.Report
			if !errors.As(err, &report) {
				t.Fatalf("missing structured source report: %v", err)
			}
			found := false
			for _, issue := range report.Diagnostics {
				if issue.Code == "openapi.comment.invalid" {
					found = true
					if issue.Source.Symbol != "example.test/metadata/dto.Unused.Value" || issue.Source.File != "example.test/metadata/dto/dto.go" || issue.Source.Line != expectedLine || issue.Source.Column != 26 || issue.Source.Kind != "declared" {
						t.Errorf("lost original dependency source: %+v", issue)
					}
					if issue.Route != "POST /bad" || len(issue.Facts) == 0 {
						t.Errorf("lost route/call evidence: %+v", issue)
					}
				}
			}
			if !found {
				t.Fatalf("dependency error was replaced by a wrapper: %+v", report.Diagnostics)
			}
		})
	}
}

// Use physical source coordinates even when a Go line directive supplies a synthetic filename.
func TestCommentSourceLineDirective(t *testing.T) {
	dir := t.TempDir()
	source := "package app\n//line misleading.go:900\n// @openapi response status=204\nfunc H() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/physical\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile(context.Background(), compiler.Options{Load: compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off", "GOPROXY=off"}}, Frontends: []compiler.Frontend{{Name: "physical-source-v1", Match: func(f compiler.Function) bool { return true }}}})
	if err != nil {
		t.Fatal(err)
	}
	index := result.Bundle.Index()
	if len(index) != 1 || index[0].Source.File != "real.go" || index[0].Source.Line != 4 {
		t.Fatalf("synthetic filename escaped into source evidence: %+v", index)
	}
	if len(index[0].Facts) != 1 || index[0].Facts[0].File != "real.go" || index[0].Facts[0].Line != 3 {
		t.Fatalf("wrong physical directive source: %+v", index[0].Facts)
	}
}

// Source-level type and range errors must expose stable codes and the actual imported field, not only formatted text.
func TestImportedSemanticDiagnosticSources(t *testing.T) {
	for _, sample := range []struct{ name, constraint, code string }{
		{"type", "minimum=1", "openapi.comment.type"},
		{"range", "minLength=3 maxLength=1", "openapi.comment.range"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			dir, dto := metadataProject(t)
			dto = strings.Replace(dto, "minLength=3", sample.constraint, 1)
			if err := os.WriteFile(filepath.Join(dir, "../dto/dto.go"), []byte(dto), 0600); err != nil {
				t.Fatal(err)
			}
			project, err := compiler.Load(context.Background(), compiler.LoadOptions{Dir: dir, Env: []string{"GOWORK=off", "GOPROXY=off"}})
			if err != nil {
				t.Fatal(err)
			}
			typ, err := project.Type("example.test/metadata/dto.Request")
			if err != nil {
				t.Fatal(err)
			}
			_, err = project.Schema(compiler.ProjectionRequest{Type: typ, Direction: compiler.Input})
			var report openapi.Report
			if !errors.As(err, &report) {
				t.Fatalf("missing structured semantic diagnostic: %v", err)
			}
			expectedLine := 0
			for i, line := range strings.Split(dto, "\n") {
				if strings.TrimSpace(line) == "Name string" {
					expectedLine = i + 1
				}
			}
			if len(report.Diagnostics) != 1 {
				t.Fatalf("unexpected semantic report: %+v", report)
			}
			issue := report.Diagnostics[0]
			if issue.Code != sample.code || issue.Source.File != "example.test/metadata/dto/dto.go" || issue.Source.Symbol != "example.test/metadata/dto.Request.Name" || issue.Source.Line != expectedLine || issue.Source.Column != 5 {
				t.Fatalf("wrong semantic source: %+v", issue)
			}
		})
	}
}
