package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/openapi-golang/openapi"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// Version compiler input accounting and projection semantics without process addresses or machine paths.
const compilerInputVersion = "openapi/compiler-inputs-v3"

// Preserve digests of parsed files so overlays and later disk changes cannot alter the loaded view.
type sourceSnapshot struct {
	digest string
	owned  bool
}

// Account for concurrent parsing and supplemental inputs using logical package paths instead of absolute directories.
type buildInputs struct {
	commentPositions map[token.Pos][]token.Pos
	mu               sync.Mutex
	remaining        int64
	snapshots        map[string]sourceSnapshot
	overlay          map[string][]byte
	entries          map[string]string
	environment      map[string]string
	profile          openapi.BuildProfile
}

// Hash bytes through an unambiguous JSON manifest instead of delimiter-based concatenation.
func inputDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Copy overlays and read the effective environment using the same Go command as packages.Load.
func prepareBuildInputs(ctx context.Context, options LoadOptions, cfg *packages.Config) (*buildInputs, error) {
	limit := options.MaxSourceBytes
	if limit == 0 {
		limit = 128 << 20
	}
	if limit < 1 {
		return nil, fmt.Errorf("openapi.load.input-budget: source byte budget must be positive")
	}
	driver := ""
	for _, entry := range cfg.Env {
		if value, ok := strings.CutPrefix(entry, "GOPACKAGESDRIVER="); ok {
			driver = value
		}
	}
	if driver != "" && driver != "off" {
		return nil, fmt.Errorf("openapi.load.driver: read-only static loading does not execute external packages drivers")
	}

	cfg.Env = append(cfg.Env, "GOPACKAGESDRIVER=off")
	inputs := &buildInputs{commentPositions: map[token.Pos][]token.Pos{}, remaining: limit, snapshots: map[string]sourceSnapshot{}, overlay: map[string][]byte{}, entries: map[string]string{}}
	for path, raw := range options.Overlay {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("openapi.load.overlay: overlay paths must be absolute")
		}
		inputs.overlay[filepath.Clean(path)] = append([]byte(nil), raw...)
	}
	cfg.Overlay = inputs.overlay
	command := exec.CommandContext(ctx, "go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "GOEXPERIMENT", "GOFLAGS", "GOMOD", "GOWORK", "GOROOT", "GOAMD64", "GO386", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM", "GOFIPS140", "CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS")
	command.Dir, command.Env = cfg.Dir, cfg.Env
	raw, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("openapi.load.environment: %w", err)
	}
	if err = json.Unmarshal(raw, &inputs.environment); err != nil {
		return nil, err
	}
	e := inputs.environment
	inheritedFlags, err := splitBuildFlags(e["GOFLAGS"])
	if err != nil {
		return nil, err
	}
	if err = validateLoadFlags(append(inheritedFlags, cfg.BuildFlags...)); err != nil {
		return nil, err
	}
	inputs.profile = openapi.BuildProfile{GoVersion: e["GOVERSION"], GOOS: e["GOOS"], GOARCH: e["GOARCH"], CGOEnabled: e["CGO_ENABLED"], GoExperiment: e["GOEXPERIMENT"], Generator: compilerInputVersion, Codec: "explicit-profile", Settings: map[string]string{"GOEXPERIMENT": e["GOEXPERIMENT"]}}
	// Record only feature selectors used by the target architecture to avoid warnings from unrelated inherited settings.
	architectureKey := map[string]string{"amd64": "GOAMD64", "386": "GO386", "arm": "GOARM", "arm64": "GOARM64", "mips": "GOMIPS", "mipsle": "GOMIPS", "mips64": "GOMIPS64", "mips64le": "GOMIPS64", "ppc64": "GOPPC64", "ppc64le": "GOPPC64", "riscv64": "GORISCV64", "wasm": "GOWASM"}[e["GOARCH"]]
	for _, key := range []string{architectureKey, "GOFIPS140"} {
		if e[key] != "" {
			inputs.profile.Settings[key] = e[key]
		}
	}
	cfg.ParseFile = func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		if _, err := inputs.snapshot(filename, src); err != nil {
			return nil, scanner.ErrorList{&scanner.Error{Pos: token.Position{Filename: filename, Line: 1, Column: 1}, Msg: err.Error()}}
		}
		file, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.AllErrors)
		inputs.captureCommentPositions(file, fset, src)
		return file, err
	}
	return inputs, nil
}

// Charge the first read against the budget and reuse already parsed file digests.
func (b *buildInputs) snapshot(path string, supplied []byte) (sourceSnapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	path = filepath.Clean(path)
	if old, ok := b.snapshots[path]; ok {
		return old, nil
	}
	raw := supplied
	if raw == nil {
		if overlay, ok := b.overlay[path]; ok {
			raw = overlay
		} else {
			file, err := os.Open(path)
			if err != nil {
				return sourceSnapshot{}, err
			}
			defer file.Close()
			raw, err = io.ReadAll(io.LimitReader(file, b.remaining+1))
			if err != nil {
				return sourceSnapshot{}, err
			}
		}
	}
	b.remaining -= int64(len(raw))
	if b.remaining < 0 {
		return sourceSnapshot{}, fmt.Errorf("openapi.load.input-budget: source inputs exceed the byte budget")
	}
	value := sourceSnapshot{digest: inputDigest(raw), owned: filepath.Base(path) == "zz_openapi.gen.go" && bytes.HasPrefix(raw, []byte("// Code generated by openapi/compiler."))}
	b.snapshots[path] = value
	return value, nil
}

// Register source by package identity and package-relative path, including resources and inactive source.
func (b *buildInputs) collect(loaded []*packages.Package, options LoadOptions, flags []string) error {
	var failure error
	modules := map[string]*packages.Module{}
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		if failure != nil {
			return
		}
		if pkg.Module != nil {
			modules[pkg.Module.Path] = pkg.Module
		}
		b.entries["package:"+pkg.PkgPath] = pkg.Name
		files := append([]string(nil), pkg.GoFiles...)
		files = append(files, pkg.OtherFiles...)
		files = append(files, pkg.IgnoredFiles...)
		files = append(files, pkg.EmbedFiles...)
		for _, path := range files {
			value, err := b.snapshot(path, nil)
			if err != nil {
				failure = err
				return
			}
			if value.owned {
				continue
			}
			rel, err := filepath.Rel(pkg.Dir, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				failure = fmt.Errorf("openapi.load.source: file is outside its package directory")
				return
			}
			b.entries["source:"+pkg.PkgPath+"/"+filepath.ToSlash(rel)] = value.digest
		}
	})
	if failure != nil {
		return failure
	}
	for _, path := range sortedKeys(modules) {
		module := modules[path]
		profile := openapi.ModuleProfile{Path: module.Path, Version: module.Version, GoVersion: module.GoVersion, Main: module.Main}
		effective := module
		if module.Replace != nil {
			effective = module.Replace
			profile.ReplacePath, profile.ReplaceVersion = effective.Path, effective.Version
			if effective.Version == "" {
				profile.ReplacePath = "local"
			}
		}
		if effective.GoMod != "" {
			raw, err := b.moduleInput(effective.GoMod)
			if err != nil {
				return err
			}
			profile.GoModDigest = inputDigest(raw)
		}
		b.profile.Modules = append(b.profile.Modules, profile)
	}
	work := b.environment["GOWORK"]
	if work != "" && work != "off" {
		b.profile.Workspace = true
		raw, err := b.workspaceInput(work)
		if err != nil {
			return err
		}
		b.entries["workspace"] = inputDigest(raw)
	}
	allFlags, err := splitBuildFlags(b.environment["GOFLAGS"])
	if err != nil {
		return err
	}
	allFlags = append(allFlags, flags...)
	normalizedFlags := []string{}
	for i := 0; i < len(allFlags); i++ {
		arg := allFlags[i]
		if arg == "-tags" && i+1 < len(allFlags) {
			i++
			arg = "-tags=" + allFlags[i]
		}
		if strings.HasPrefix(arg, "-tags=") {
			tags := strings.FieldsFunc(strings.TrimPrefix(arg, "-tags="), func(r rune) bool { return r == ',' || r == ' ' })
			sort.Strings(tags)
			b.profile.Tags = tags
			arg = "-tags=" + strings.Join(tags, ",")
		}
		normalizedFlags = append(normalizedFlags, b.logicalSetting(arg, options.Dir, modules))
	}
	rawFlags, err := json.Marshal(normalizedFlags)
	if err != nil {
		return err
	}
	b.profile.Settings["-tags"] = strings.Join(b.profile.Tags, ",")
	b.profile.BuildFlagsDigest = inputDigest(rawFlags)
	for _, key := range []string{"CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS"} {
		b.entries["environment:"+key] = b.logicalSetting(b.environment[key], options.Dir, modules)
	}
	return nil
}

// Preserve complete module declarations while replacing machine-local paths with replaced module identities.
func (b *buildInputs) moduleInput(path string) ([]byte, error) {
	raw, err := b.controlInput(path)
	if err != nil {
		return nil, err
	}
	file, err := modfile.Parse(path, raw, nil)
	if err != nil {
		return nil, err
	}
	for _, replace := range append([]*modfile.Replace(nil), file.Replace...) {
		if replace.New.Version == "" {
			if err := file.AddReplace(replace.Old.Path, replace.Old.Version, "./local/"+replace.Old.Path, ""); err != nil {
				return nil, err
			}
		}
	}
	return file.Format()
}

// Record workspace module identities and version rules without machine-local use or replace paths.
func (b *buildInputs) workspaceInput(path string) ([]byte, error) {
	raw, err := b.controlInput(path)
	if err != nil {
		return nil, err
	}
	file, err := modfile.ParseWork(path, raw, nil)
	if err != nil {
		return nil, err
	}
	uses := []*modfile.Use{}
	for _, use := range file.Use {
		dir := use.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(path), dir)
		}
		data, err := b.controlInput(filepath.Join(dir, "go.mod"))
		if err != nil {
			return nil, err
		}
		module, err := modfile.Parse("go.mod", data, nil)
		if err != nil {
			return nil, err
		}
		if module.Module == nil {
			return nil, fmt.Errorf("openapi.load.workspace: workspace module has no identity")
		}
		uses = append(uses, &modfile.Use{Path: "./modules/" + module.Module.Mod.Path})
	}
	file.SetUse(uses)
	for _, replace := range append([]*modfile.Replace(nil), file.Replace...) {
		if replace.New.Version == "" {
			if err := file.AddReplace(replace.Old.Path, replace.Old.Version, "./local/"+replace.Old.Path, ""); err != nil {
				return nil, err
			}
		}
	}
	file.Cleanup()
	return modfile.Format(file.Syntax), nil
}

// Bound module control files by the same input budget and read only explicitly loaded or declared files.
func (b *buildInputs) controlInput(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	raw, err := io.ReadAll(io.LimitReader(file, b.remaining+1))
	if err != nil {
		return nil, err
	}
	b.remaining -= int64(len(raw))
	if b.remaining < 0 {
		return nil, fmt.Errorf("openapi.load.input-budget: module inputs exceed the byte budget")
	}
	return raw, nil
}

// Replace known absolute directories in settings with project, module, and toolchain identities.
func (b *buildInputs) logicalSetting(value, dir string, modules map[string]*packages.Module) string {
	roots := map[string]string{}
	for _, module := range modules {
		effective := module
		if module.Replace != nil {
			effective = module.Replace
		}
		if effective.GoMod != "" {
			roots[filepath.Dir(effective.GoMod)] = "$MODULE_CONFIG/" + module.Path
		}
		if effective.Dir != "" {
			roots[effective.Dir] = "$MODULE/" + module.Path
		}
	}
	absolute, _ := filepath.Abs(dir)
	roots[absolute] = "$PROJECT"
	if root := b.environment["GOROOT"]; root != "" {
		roots[root] = "$GOROOT"
	}
	paths := sortedKeys(roots)
	sort.SliceStable(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, path := range paths {
		value = strings.ReplaceAll(value, path, roots[path])
	}
	return value
}

// Split whole-argument GOFLAGS quotes without interpreting escapes or executing commands.
func splitBuildFlags(text string) ([]string, error) {
	var result []string
	for {
		text = strings.TrimLeft(text, " \t\r\n")
		if text == "" {
			return result, nil
		}
		if text[0] == '\'' || text[0] == '"' {
			quote := text[0]
			end := strings.IndexByte(text[1:], quote)
			if end < 0 {
				return nil, fmt.Errorf("openapi.load.flags: unterminated quote in build flags")
			}
			result = append(result, text[1:end+1])
			text = text[end+2:]
			continue
		}
		end := strings.IndexAny(text, " \t\r\n")
		if end < 0 {
			return append(result, text), nil
		}
		result = append(result, text[:end])
		text = text[end:]
	}
}

// Combine actual inputs, declared configuration, budgets, and projected output without reflecting callback addresses.
func (p *Project) fingerprint(options Options, data openapi.BundleData) (string, error) {
	configuration := map[string]json.RawMessage{}
	for name, raw := range options.Configuration {
		if name == "" || !json.Valid(raw) {
			return "", fmt.Errorf("openapi.fingerprint.configuration: invalid configuration name or JSON")
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return "", err
		}
		configuration[name] = compact.Bytes()
	}
	manifest := struct {
		Inputs                                      map[string]string
		Configuration                               map[string]json.RawMessage
		MaxDepth, MaxPaths, MaxCalls, MaxIterations int
		Contract                                    openapi.BundleData
	}{p.inputs.entries, configuration, options.MaxDepth, options.MaxPaths, options.MaxCalls, options.MaxIterations, data}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return inputDigest(raw), nil
}

// Store only a digest of custom configuration in the profile, not potentially sensitive raw JSON.
func configurationDigest(configuration map[string]json.RawMessage) (string, error) {
	if len(configuration) == 0 {
		return "", nil
	}
	for key, value := range configuration {
		if key == "" || !json.Valid(value) {
			return "", fmt.Errorf("openapi.fingerprint.configuration: invalid configuration name or JSON")
		}
	}
	raw, err := json.Marshal(configuration)
	if err != nil {
		return "", err
	}
	return inputDigest(raw), nil
}

// Disallow module writes, executable tool wrappers, and overlays outside the SDK snapshot mechanism.
func validateLoadFlags(flags []string) error {
	for i := 0; i < len(flags); i++ {
		key, value, assigned := strings.Cut(flags[i], "=")
		if !assigned && (key == "-mod" || key == "-toolexec" || key == "-overlay") && i+1 < len(flags) {
			i++
			value = flags[i]
		}
		if key == "-toolexec" || key == "-overlay" {
			return fmt.Errorf("openapi.load.flags: %s is not supported by read-only SDK loading", key)
		}
		if key == "-mod" && value != "readonly" && value != "vendor" {
			return fmt.Errorf("openapi.load.flags: module mode must be readonly or vendor")
		}
	}
	return nil
}
