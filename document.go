package openapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openapi-golang/openapi/internal/validate"
	"github.com/openapi-golang/openapi/spec"
)

// 仅接收标准化 OpenAPI 路径，不识别任何框架路由语法。
// Accept normalized OpenAPI paths without framework route syntax.
type Route struct {
	// 显式声明此路由的请求媒体类型；空切片表示尚未确定。
	// Explicitly declare request media types for this route; an empty slice means unresolved.
	RequestMediaTypes []string
	Method            string
	Path              string
	OperationKey      OperationKey
	Source            Source
	Extensions        spec.Extensions
}

// 配置核心文档信息与类型化高级表达入口。
// Configure document metadata and typed advanced extensions.
type Config struct {
	// 运行时链接可选择核对当前程序；离线跨目标导出默认不启用。
	// Optionally verify the current executable when linking; leave disabled for cross-target offline exports.
	VerifyRuntimeBuild bool

	Title       string
	Version     string
	Description string
	Servers     []spec.Server
	Tags        []spec.Tag
	Security    spec.Optional[[]spec.SecurityRequirement]
	Extensions  spec.Extensions
	Configure   func(*spec.OpenAPI) error
	// 将相同离线资源与预算用于引用裁剪和最终规范检查。
	// Use the same offline resources and budgets for reference pruning and final specification checks.
	Validation CheckOptions
}

// 保存已验证的不可变 JSON 与来源报告。
// Store validated immutable JSON and its provenance report.
type Document struct {
	data   string
	report Report
}

// 返回缓存 JSON 的防御性副本。
// Return a defensive copy of cached JSON.
func (d *Document) JSON() []byte { return []byte(d.data) }

// 返回来源报告的防御性副本。
// Return a defensive copy of provenance and diagnostics.
func (d *Document) Report() Report { return copyJSON(d.report) }

// 原子写入目标文件；失败清除本次临时文件。
// Write atomically and clean up the temporary file on failure.
func (d *Document) WriteFile(path string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".openapi-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.WriteString(d.data); err != nil {
		file.Close()
		return err
	}
	if err = file.Chmod(0644); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// 在中立路由与静态模板间链接并验证；不读取源码，不执行 handler。
// Link neutral routes to static templates without reading source or executing handlers.
func Build(bundle Bundle, routes []Route, cfg Config) (*Document, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	data := bundle.Snapshot()
	if _, err := NewBundle(data); err != nil {
		return nil, err
	}
	report := Report{Diagnostics: append([]Diagnostic{}, data.Diagnostics...)}
	if cfg.VerifyRuntimeBuild {
		report.Diagnostics = append(report.Diagnostics, CheckRuntimeBuild(data.Profile).Diagnostics...)
		if report.HasErrors() {
			return nil, report
		}
	}
	add := func(code, msg, fix string) {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: Error, Message: msg, Fix: fix})
	}
	index := map[OperationKey]Template{}
	for _, t := range data.Templates {
		index[t.Key] = t
	}
	doc := spec.OpenAPI{OpenAPI: "3.2.0", JSONSchemaDialect: spec.DefaultDialect, Info: spec.Info{Title: cfg.Title, Version: cfg.Version, Description: cfg.Description}, Paths: map[string]*spec.PathItem{}, Components: &data.Components, Servers: copyJSON(cfg.Servers), Tags: copyJSON(cfg.Tags), Security: spec.Optional[[]spec.SecurityRequirement]{Present: cfg.Security.Present, Value: copyJSON(cfg.Security.Value)}, Extensions: copyJSON(cfg.Extensions)}
	ordered := append([]Route(nil), routes...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Path != ordered[j].Path {
			return ordered[i].Path < ordered[j].Path
		}
		return ordered[i].Method < ordered[j].Method
	})
	seen := map[string]bool{}
	for _, route := range ordered {
		key := route.Method + " " + route.Path
		if seen[key] {
			add("openapi.route.duplicate", key, "移除重复的中立路由")
			continue
		}
		seen[key] = true
		template, ok := index[route.OperationKey]
		if !ok {
			add("openapi.route.unresolved", key, "提供可匹配的 OperationKey")
			continue
		}
		op, diagnostics, facts, err := linkConditionalTemplate(template, route)
		if err != nil {
			add("openapi.condition.unresolved", key, err.Error())
			continue
		}
		if route.Method == "HEAD" {
			if err := projectHEADResponses(&op, data.Components); err != nil {
				add("openapi.head.unresolved", key, err.Error())
				continue
			}
		}
		report.Diagnostics = append(report.Diagnostics, diagnostics...)
		report.Facts = append(report.Facts, facts...)
		if op.OperationID == "" {
			sum := sha256.Sum256([]byte(key))
			op.OperationID = strings.ToLower(route.Method) + "_" + hex.EncodeToString(sum[:8])
		}
		if len(op.Responses) == 0 {
			add("openapi.response.missing", key, "补全可分析响应或显式兜底契约")
		}
		// 路径模板本身证明命名参数存在；缺少读取证据时只声明字符串。
		// Derive string path parameters from the route template when no read evidence exists.
		for _, name := range pathParameters(route.Path) {
			found := false
			for _, p := range op.Parameters {
				if p.Value != nil && p.Value.In == "path" && p.Value.Name == name {
					found = true
				}
			}
			if !found {
				op.Parameters = append(op.Parameters, spec.Inline(spec.Parameter{Name: name, In: "path", Required: true, Schema: spec.Typed("string")}))
			}
		}
		path := doc.Paths[route.Path]
		if path == nil {
			path = &spec.PathItem{}
			doc.Paths[route.Path] = path
		}
		if err := putOperation(path, route.Method, &op); err != nil {
			add("openapi.method.invalid", key, err.Error())
		}
		if len(route.Extensions) > 0 {
			if op.Extensions == nil {
				op.Extensions = spec.Extensions{}
			}
			for k, v := range route.Extensions {
				op.Extensions[k] = bytes.Clone(v)
			}
		}
	}
	if report.HasErrors() {
		return nil, report
	}
	if cfg.Configure != nil {
		if err := cfg.Configure(&doc); err != nil {
			return nil, err
		}
	}
	if err := pruneSchemas(&doc, cfg.Validation); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	checked := CheckWithOptions(raw, cfg.Validation)
	report.Diagnostics = append(report.Diagnostics, checked.Diagnostics...)
	if report.HasErrors() {
		return nil, report
	}
	return &Document{data: string(raw) + "\n", report: report}, nil
}

// 从标准花括号模板提取路径参数；不处理框架的特殊前缀。
// Extract standard brace parameters without interpreting framework prefixes.
func pathParameters(path string) []string {
	var names []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			break
		}
		path = path[start+1:]
		end := strings.IndexByte(path, '}')
		if end < 0 {
			break
		}
		names = append(names, path[:end])
		path = path[end+1:]
	}
	return names
}

// 仅按 OpenAPI 固定方法和扩展方法分派。
// Dispatch fixed and additional OpenAPI methods.
func putOperation(p *spec.PathItem, method string, op *spec.Operation) error {
	switch method {
	case "GET":
		p.Get = op
	case "PUT":
		p.Put = op
	case "POST":
		p.Post = op
	case "DELETE":
		p.Delete = op
	case "OPTIONS":
		p.Options = op
	case "HEAD":
		p.Head = op
	case "PATCH":
		p.Patch = op
	case "TRACE":
		p.Trace = op
	case "QUERY":
		p.Query = op
	default:
		if method == "" || strings.ContainsAny(method, " \t\r\n") {
			return fmt.Errorf("提供合法 HTTP 方法")
		}
		if p.AdditionalOperations == nil {
			p.AdditionalOperations = map[string]*spec.Operation{}
		}
		p.AdditionalOperations[method] = op
	}
	return nil
}

// 裁剪不可达 Schema，保留高级对象引用到的类型闭包。
// Retain only reachable Schemas, including references from advanced objects.
func pruneSchemas(doc *spec.OpenAPI, options CheckOptions) error {
	if doc.Components == nil {
		return nil
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	names, issues := validate.ReachableSchemas(raw, options.internal())
	if len(issues) != 0 {
		return issuesReport(issues)
	}
	retained := make(map[string]*spec.Schema, len(names))
	for _, name := range names {
		if schema, ok := doc.Components.Schemas[name]; ok {
			retained[name] = schema
		}
	}
	doc.Components.Schemas = retained
	return nil
}
