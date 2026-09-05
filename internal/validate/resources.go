package validate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// 保存显式离线输入及预算，所有零值使用有界默认值。
// Store explicit offline inputs and budgets with bounded defaults for zero values.
type Options struct {
	BaseURI                               string
	Resources                             map[string][]byte
	ExampleResources                      map[string][]byte
	MaxBytes, MaxResources, MaxReferences int
	MaxIndexBytes, MaxNormalizedBytes     int
}

// 保存已解码文档的检索地址和标准对象上下文。
// Store decoded documents with their retrieval addresses and standard object contexts.
type parsedResource struct {
	value            any
	path, base, role string
}

// 保存本次调用的独立资源集合，不共享调用方的可变 JSON 数据。
// Keep a per-call resource set without sharing mutable JSON data with the caller.
type resourceSet struct {
	documents []parsedResource
	graph     *referenceGraph
}

// 验证显式资源配置，并应用默认预算。
// Validate explicit resource options and apply default budgets.
func normalizeOptions(options Options) (Options, error) {
	if options.MaxBytes == 0 {
		options.MaxBytes = 8 << 20
	}
	if options.MaxResources == 0 {
		options.MaxResources = 64
	}
	if options.MaxReferences == 0 {
		options.MaxReferences = 10000
	}
	if options.MaxIndexBytes == 0 {
		options.MaxIndexBytes = 16 << 20
	}
	if options.MaxNormalizedBytes == 0 {
		options.MaxNormalizedBytes = 16 << 20
	}
	if options.MaxBytes < 1 || options.MaxResources < 1 || options.MaxReferences < 1 || options.MaxIndexBytes < 1 || options.MaxNormalizedBytes < 1 {
		return options, fmt.Errorf("预算必须大于零")
	}
	if options.BaseURI == "" {
		options.BaseURI = "https://openapi.invalid/document.json"
	}
	if len(options.BaseURI) > options.MaxIndexBytes {
		return options, errIndexBudget
	}
	base, err := retrievalURI(options.BaseURI)
	if err != nil {
		return options, err
	}
	options.BaseURI = base
	return options, nil
}

// 检索地址必须是无片段的绝对 URI，但不会据此读取地址内容。
// Require absolute, fragment-free retrieval URIs without loading their contents.
func retrievalURI(value string) (string, error) {
	u, err := parseURIReference(value)
	if err != nil || !u.IsAbs() || strings.Contains(value, "#") {
		return "", fmt.Errorf("检索地址必须是无片段的绝对 URI：%s", value)
	}
	return resourceURI(u), nil
}

// 用稳定次序读取资源键，确保诊断可复现。
// Read resource keys in stable order for reproducible diagnostics.
func resourceKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// 在任何解析前限制累计字节数和显式输入数量。
// Limit aggregate bytes and explicit input counts before parsing anything.
func prepareResources(raw []byte, options Options) (*resourceSet, []Issue) {
	return prepareSchemaResources(raw, options, true)
}

// 共享离线索引支持规范检查和独立 Schema 消费，两者保留各自的根类型要求。
// Shared offline indexing supports document checks and standalone schema consumers with distinct root requirements.
func prepareSchemaResources(raw []byte, options Options, requireOpenAPI bool) (*resourceSet, []Issue) {
	graph := newReferenceGraph()
	options, err := normalizeOptions(options)
	if err != nil {
		code := "options"
		if errors.Is(err, errIndexBudget) {
			code = "budget"
		}
		graph.add(code, "#", err.Error())
		return nil, graph.issues
	}
	if len(options.Resources) >= options.MaxResources || len(options.ExampleResources) > options.MaxResources-1-len(options.Resources) {
		graph.add("budget", "#", "主文档与预载资源数量超过预算")
		return nil, graph.issues
	}
	graph.budget = byteBudget{remaining: options.MaxIndexBytes}
	graph.maxNormalizedBytes = options.MaxNormalizedBytes
	if !graph.spend(len(options.BaseURI)) {
		return nil, graph.issues
	}
	for _, values := range []map[string][]byte{options.Resources, options.ExampleResources} {
		for key := range values {
			if !graph.spend(len(key)) {
				return nil, graph.issues
			}
		}
	}
	remaining := options.MaxBytes
	consume := func(size int) bool {
		if size > remaining {
			return false
		}
		remaining -= size
		return true
	}
	if !consume(len(raw)) {
		graph.add("budget", "#", "主文档超过总字节预算")
		return nil, graph.issues
	}
	for _, values := range []map[string][]byte{options.Resources, options.ExampleResources} {
		for _, key := range resourceKeys(values) {
			if !consume(len(values[key])) {
				graph.add("budget", key, "预载内容超过累计字节预算")
				return nil, graph.issues
			}
		}
	}
	graph.maxReferences = options.MaxReferences
	graph.maxResources = options.MaxResources - len(options.ExampleResources)
	set := &resourceSet{graph: graph}
	count := 0
	addDocument := func(data []byte, base, path string, primary bool) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		value, err := decode(decoder, 0, &count)
		if err != nil {
			code := "json"
			if errors.Is(err, errJSONBudget) {
				code = "budget"
			}
			graph.add(code, path, err.Error())
			return
		}
		if _, err = decoder.Token(); err != io.EOF {
			graph.add("json", path, "文档末尾有额外内容")
			return
		}
		object, isObject := value.(map[string]any)
		_, isBoolean := value.(bool)
		role := "schema"
		if (primary && requireOpenAPI) || has(object, "openapi") {
			role = "root"
		}
		if primary && requireOpenAPI && !isObject {
			graph.add("root", path, "主文档根必须是对象")
			return
		}
		if !isObject && !isBoolean {
			graph.add("resource.type", path, "预载规范必须是完整 OpenAPI 对象或 JSON Schema 对象/布尔值")
			return
		}
		set.documents = append(set.documents, parsedResource{value: value, path: path, base: base, role: role})
	}
	addDocument(raw, options.BaseURI, "#", true)
	seenBases := map[string]bool{options.BaseURI: true}
	for _, key := range resourceKeys(options.Resources) {
		base, err := retrievalURI(key)
		if err != nil {
			graph.add("resource.uri", key, err.Error())
			continue
		}
		if seenBases[base] {
			graph.add("resource.duplicate", base, "检索地址归一化后重复")
			continue
		}
		if !graph.spend(len(base)) {
			return nil, graph.issues
		}
		seenBases[base] = true
		addDocument(options.Resources[key], base, base+"#", false)
	}
	if len(graph.issues) > 0 {
		return nil, sortedIssues(graph.issues)
	}
	for _, entry := range set.documents {
		if !graph.spend(len(entry.path)) {
			break
		}
		graph.collect(entry.value, entry.path, entry.role, entry.base, entry.path, true)
	}
	for _, key := range resourceKeys(options.ExampleResources) {
		base, err := retrievalURI(key)
		if err != nil {
			graph.add("resource.uri", key, err.Error())
			continue
		}
		if graph.resources[base] != nil || graph.exampleResources[base] {
			graph.add("resource.duplicate", key, "示例检索地址与已提供资源重复")
			continue
		}
		if !graph.spend(len(base)) {
			return nil, graph.issues
		}
		graph.exampleResources[base] = true
	}
	if len(graph.issues) > 0 {
		return nil, sortedIssues(graph.issues)
	}
	return set, nil
}

// 检查全部明确提供的规范资源；示例原始字节仅用于证明已离线提供。
// Check all explicitly supplied specification resources; raw examples only establish offline availability.
func CheckWithOptions(raw []byte, options Options) []Issue {
	set, issues := prepareResources(raw, options)
	if len(issues) > 0 {
		return issues
	}
	for _, entry := range set.documents {
		object, _ := entry.value.(map[string]any)
		if set.graph.budget.exceeded {
			break
		}
		c := checker{root: object, ids: map[string]string{}, graph: set.graph}
		c.walk(entry.value, entry.path, entry.role, 0)
		if entry.role == "root" {
			c.checkTags()
		}
		// 外部文档内标签诊断需要带上资源位置。
		// Include resource locations in tag diagnostics from external documents.
		for _, issue := range c.issues {
			if entry.path != "#" && strings.HasPrefix(issue.Path, "#") {
				if !set.graph.spend(len(entry.path), len(issue.Path)-1) {
					break
				}
				issue.Path = entry.path + strings.TrimPrefix(issue.Path, "#")
			}
			issues = append(issues, issue)
		}
	}
	set.graph.checkReferences()
	issues = append(issues, set.graph.issues...)
	return sortedIssues(issues)
}

// 对同位置的错误继续按编码及消息排序，避免映射遍历造成差异。
// Sort diagnostics sharing a location by code and message to remove map-order differences.
func sortedIssues(issues []Issue) []Issue {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Message < issues[j].Message
	})
	return issues
}

// 识别主文档顶层组件名，目标指向组件内部时仍保留整个组件。
// Identify root component names and retain the entire component when a reference targets its interior.
func mainSchemaName(path string) string {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	encoded := strings.SplitN(strings.TrimPrefix(path, prefix), "/", 2)[0]
	return strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
}

// 基于与 Check 相同的资源图计算闭包，不解释示例或扩展中的数据引用。
// Compute reachability using the same resource graph as Check, excluding references inside examples and extensions.
func ReachableSchemas(raw []byte, options Options) ([]string, []Issue) {
	set, issues := prepareResources(raw, options)
	if len(issues) > 0 {
		return nil, issues
	}
	graph := set.graph
	bySchema := map[string][]referenceUse{}
	var queue []referenceUse
	for _, use := range graph.uses {
		if name := mainSchemaName(use.owner); name != "" {
			bySchema[name] = append(bySchema[name], use)
		} else {
			queue = append(queue, use)
		}
	}
	needed := map[string]bool{}
	for i := 0; i < len(queue); i++ {
		if graph.budget.exceeded {
			break
		}
		use := queue[i]
		target, code, message := graph.target(use)
		if code != "" {
			graph.add(code, use.path, message)
			continue
		}
		if target == nil {
			continue
		}
		name := mainSchemaName(target.path)
		if name != "" && !needed[name] {
			needed[name] = true
			queue = append(queue, bySchema[name]...)
		}
	}
	if len(graph.issues) > 0 {
		return nil, sortedIssues(graph.issues)
	}
	names := make([]string, 0, len(needed))
	for name := range needed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
