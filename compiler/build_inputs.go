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

// 标识编译器输入清单与投影算法的版本，不使用进程地址或机器路径。
// Version compiler input accounting and projection semantics without process addresses or machine paths.
const compilerInputVersion = "openapi/compiler-inputs-v3"

// 保存实际解析的文件摘要，避免 overlay 或并发磁盘修改改变已加载视图。
// Preserve digests of parsed files so overlays and later disk changes cannot alter the loaded view.
type sourceSnapshot struct {
	digest string
	owned  bool
}

// 对并发解析和补充输入统一计费，记录逻辑包路径而不是绝对目录。
// Account for concurrent parsing and supplemental inputs using logical package paths instead of absolute directories.
type buildInputs struct {
	mu          sync.Mutex
	remaining   int64
	snapshots   map[string]sourceSnapshot
	overlay     map[string][]byte
	entries     map[string]string
	environment map[string]string
	profile     openapi.BuildProfile
}

// 哈希字节采用明确长度的 JSON 清单，不依赖拼接分隔符。
// Hash bytes through an unambiguous JSON manifest instead of delimiter-based concatenation.
func inputDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// 复制覆盖输入，并使用与 packages.Load 相同的 Go 命令读取有效环境。
// Copy overlays and read the effective environment using the same Go command as packages.Load.
func prepareBuildInputs(ctx context.Context, options LoadOptions, cfg *packages.Config) (*buildInputs, error) {
	limit := options.MaxSourceBytes
	if limit == 0 {
		limit = 128 << 20
	}
	if limit < 1 {
		return nil, fmt.Errorf("openapi.load.input-budget: 源码字节预算必须为正")
	}
	driver := ""
	for _, entry := range cfg.Env {
		if value, ok := strings.CutPrefix(entry, "GOPACKAGESDRIVER="); ok {
			driver = value
		}
	}
	if driver != "" && driver != "off" {
		return nil, fmt.Errorf("openapi.load.driver: 只读静态加载不执行外部 packages driver")
	}

	cfg.Env = append(cfg.Env, "GOPACKAGESDRIVER=off")
	inputs := &buildInputs{remaining: limit, snapshots: map[string]sourceSnapshot{}, overlay: map[string][]byte{}, entries: map[string]string{}}
	for path, raw := range options.Overlay {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("openapi.load.overlay: 覆盖路径必须为绝对路径")
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
	// 只记录目标架构实际使用的特性设置，避免继承其他架构环境造成误报。
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
		return parser.ParseFile(fset, filename, src, parser.ParseComments|parser.AllErrors)
	}
	return inputs, nil
}

// 首次读入时扣减预算；已解析文件直接使用其确定的摘要。
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
		return sourceSnapshot{}, fmt.Errorf("openapi.load.input-budget: 输入源码超过字节预算")
	}
	value := sourceSnapshot{digest: inputDigest(raw), owned: filepath.Base(path) == "zz_openapi.gen.go" && bytes.HasPrefix(raw, []byte("// Code generated by openapi/compiler."))}
	b.snapshots[path] = value
	return value, nil
}

// 用包身份和包内路径登记源码，资源或非当前构建源码同样不能遗漏。
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
				failure = fmt.Errorf("openapi.load.source: 文件不在所属包目录中")
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

// 对模块声明保留全部语义，唯一替换的机器路径使用被替换的模块身份。
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

// 工作区只记录模块身份与版本规则，不输出 use 或 replace 的本机目录。
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
			return nil, fmt.Errorf("openapi.load.workspace: 工作区模块缺少身份")
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

// 模块控制文件受同一输入预算约束，只读取明确加载或声明的文件。
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
		return nil, fmt.Errorf("openapi.load.input-budget: 模块输入超过字节预算")
	}
	return raw, nil
}

// 使用项目、模块和工具链身份替换配置中的已知绝对目录。
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

// GOFLAGS 仅在词首支持整体引号，不解释转义或执行命令。
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
				return nil, fmt.Errorf("openapi.load.flags: 构建参数引号不完整")
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

// 结合实际输入、配置声明、预算和已投影结果，避免遗漏不可反射的回调结果。
// Combine actual inputs, declared configuration, budgets, and projected output without reflecting callback addresses.
func (p *Project) fingerprint(options Options, data openapi.BundleData) (string, error) {
	configuration := map[string]json.RawMessage{}
	for name, raw := range options.Configuration {
		if name == "" || !json.Valid(raw) {
			return "", fmt.Errorf("openapi.fingerprint.configuration: 配置名或 JSON 无效")
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return "", err
		}
		configuration[name] = compact.Bytes()
	}
	manifest := struct {
		Inputs                       map[string]string
		Configuration                map[string]json.RawMessage
		MaxDepth, MaxPaths, MaxCalls int
		Contract                     openapi.BundleData
	}{p.inputs.entries, configuration, options.MaxDepth, options.MaxPaths, options.MaxCalls, data}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return inputDigest(raw), nil
}

// 对写入 profile 的自定义配置只保留摘要，不嵌入可含敏感值的原始 JSON。
// Store only a digest of custom configuration in the profile, not potentially sensitive raw JSON.
func configurationDigest(configuration map[string]json.RawMessage) (string, error) {
	if len(configuration) == 0 {
		return "", nil
	}
	for key, value := range configuration {
		if key == "" || !json.Valid(value) {
			return "", fmt.Errorf("openapi.fingerprint.configuration: 配置名或 JSON 无效")
		}
	}
	raw, err := json.Marshal(configuration)
	if err != nil {
		return "", err
	}
	return inputDigest(raw), nil
}

// 构建参数不能启用模块写入、执行包装器或绕过 SDK 的 overlay 快照。
// Disallow module writes, executable tool wrappers, and overlays outside the SDK snapshot mechanism.
func validateLoadFlags(flags []string) error {
	for i := 0; i < len(flags); i++ {
		key, value, assigned := strings.Cut(flags[i], "=")
		if !assigned && (key == "-mod" || key == "-toolexec" || key == "-overlay") && i+1 < len(flags) {
			i++
			value = flags[i]
		}
		if key == "-toolexec" || key == "-overlay" {
			return fmt.Errorf("openapi.load.flags: %s 不适用于只读 SDK 加载", key)
		}
		if key == "-mod" && value != "readonly" && value != "vendor" {
			return fmt.Errorf("openapi.load.flags: 模块模式必须为 readonly 或 vendor")
		}
	}
	return nil
}
