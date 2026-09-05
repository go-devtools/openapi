package validate

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// 保存真实规范节点的位置、上下文和所属文档。
type referenceNode struct {
	value                      any
	path, role, base, document string
}

// 保存每次引用使用，而非仅按引用文本去重。
type referenceUse struct {
	value, path, base, role, owner string
}

// 对已提供内容建立离线资源图，不包含文件或网络加载器。
type referenceGraph struct {
	nodes                 map[string]*referenceNode
	resources             map[string]*referenceNode
	anchors               map[string]*referenceNode
	uses                  []referenceUse
	issues                []Issue
	budget                byteBudget
	maxNormalizedBytes    int
	maxReferences         int
	maxResources          int
	resourceNodes         map[string]bool
	exampleResources      map[string]bool
	resourceLimitReported bool
	limitReported         bool
}

// 建立默认有界引用图，资源身份只在本次检查内生效。
func newReferenceGraph() *referenceGraph {
	return &referenceGraph{budget: byteBudget{remaining: 16 << 20}, maxNormalizedBytes: 16 << 20, nodes: map[string]*referenceNode{}, resources: map[string]*referenceNode{}, anchors: map[string]*referenceNode{}, maxReferences: 10000, maxResources: 64, resourceNodes: map[string]bool{}, exampleResources: map[string]bool{}}
}

// 记录与普通检查一致的错误编码。
func (g *referenceGraph) add(code, path, message string) {
	if !g.spend(len(code), len(path), len(message), 128) {
		return
	}
	g.issues = append(g.issues, Issue{Code: "openapi.spec." + code, Path: path, Message: message, Fix: "修正引用或显式提供离线资源；检查器不会读取文件或网络"})
}

// 拒绝 URI 中未编码的非法字符，同时保留合法的相对引用。
func parseURIReference(value string) (*url.URL, error) {
	for _, c := range value {
		if c <= 0x20 || c >= 0x7f || strings.ContainsRune("<>\"{}|\\^`", c) {
			return nil, fmt.Errorf("URI 包含未编码字符")
		}
	}
	return url.Parse(value)
}

// 解析相对引用并返回绝对 URI；不触发任何 I/O。
func absoluteReference(base, value string) (*url.URL, error) {
	u, err := parseURIReference(value)
	if err != nil {
		return nil, err
	}
	b, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	return b.ResolveReference(u), nil
}

// 资源身份不包含片段，空片段与无片段使用同一键。
func resourceURI(u *url.URL) string {
	copy := *u
	copy.Fragment, copy.RawFragment = "", ""
	copy.Scheme, copy.Host = strings.ToLower(copy.Scheme), strings.ToLower(copy.Host)
	return copy.String()
}

// 固定遍历顺序，使重复身份的诊断位置可复现。
// Keeps traversal deterministic so duplicate-identity diagnostics are reproducible.
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// 映射规范字典的元素上下文；字典键本身不是关键字。
func dictionaryRole(role string) string {
	switch role {
	case "schemas":
		return "schema"
	case "paths", "pathItems", "webhooks":
		return "path"
	case "responses":
		return "response"
	case "parameters":
		return "parameter"
	case "headers":
		return "header"
	case "mediaTypes":
		return "media"
	case "examples":
		return "example"
	case "securitySchemes":
		return "security"
	case "links":
		return "link"
	case "callbacks":
		return "callback"
	case "encodings":
		return "encoding"
	case "requestBodies":
		return "requestBody"
	case "additionalOperations":
		return "operation"
	}
	return ""
}

// 只有支持扩展的字典将 x- 键解释为扩展，Schema 属性名始终保留。
func dictionaryExtension(role, key string) bool {
	return (role == "paths" || role == "responses") && strings.HasPrefix(key, "x-")
}

// 标识允许 Reference Object 或 Schema 引用的标准位置。
func referenceRole(role string) bool {
	switch role {
	case "schema", "path", "response", "parameter", "header", "requestBody", "media", "example", "security", "link", "callback":
		return true
	}
	return false
}

// 建立 URI、锚点和规范对象索引，示例及扩展内容不作为引用指令。
func (g *referenceGraph) collect(v any, path, role, base, document string, root bool) {
	if g.budget.exceeded {
		return
	}
	if a, ok := v.([]any); ok {
		if role == "schemaArray" {
			role = "schema"
		}
		for i, item := range a {
			if g.budget.exceeded {
				return
			}
			g.collect(item, g.childPath(path, strconv.Itoa(i)), role, base, document, false)
		}
		return
	}
	m, object := v.(map[string]any)
	if singular := dictionaryRole(role); singular != "" {
		for _, key := range sortedKeys(m) {
			if g.budget.exceeded {
				return
			}
			if !dictionaryExtension(role, key) {
				g.collect(m[key], g.childPath(path, key), singular, base, document, false)
			}
		}
		return
	}
	if !object {
		if _, boolean := v.(bool); role != "schema" || !boolean {
			return
		}
	}
	node := &referenceNode{value: v, path: path, role: role, base: base, document: document}
	g.nodes[path] = node
	retrieval := base
	if role == "root" && has(m, "$self") {
		value, valid := m["$self"].(string)
		u, err := absoluteReference(base, value)
		if !valid || err != nil {
			g.add("self", g.childPath(path, "$self"), "$self 必须是合法 URI-reference")
		} else {
			base = resourceURI(u)
			if !g.spend(len(base)) {
				return
			}
		}
	}
	identified := false
	if role == "schema" && has(m, "$id") {
		value, valid := m["$id"].(string)
		u, err := absoluteReference(base, value)
		if !valid || err != nil || u.Fragment != "" {
			g.add("schema.id", g.childPath(path, "$id"), "$id 必须是无非空片段的 URI-reference")
		} else {
			base, identified = resourceURI(u), true
			if !g.spend(len(base)) {
				return
			}
		}
	}
	node.base = base
	if root {
		g.addResource(retrieval, node)
	}
	if root || identified {
		g.addResource(base, node)
	}
	if role == "schema" {
		for _, key := range []string{"$anchor", "$dynamicAnchor"} {
			if g.budget.exceeded {
				return
			}
			if !has(m, key) {
				continue
			}
			name, valid := m[key].(string)
			if !valid || !validAnchor(name) {
				g.add("schema.anchor", g.childPath(path, key), "锚点必须以字母或下划线开头，并只包含字母、数字、点、横线或下划线")
				continue
			}
			if !g.spend(len(base), 1, len(name)) {
				return
			}
			identity := base + "#" + name
			if previous := g.anchors[identity]; previous != nil && previous.path != path {
				g.add("schema.anchor.duplicate", g.childPath(path, key), "锚点与 "+previous.path+" 重复")
			} else {
				g.anchors[identity] = node
			}
		}
		if has(m, "$dynamicRef") {
			g.addUse(m["$dynamicRef"], g.childPath(path, "$dynamicRef"), node, "schema")
		}
	}
	if has(m, "$ref") && referenceRole(role) {
		g.addUse(m["$ref"], g.childPath(path, "$ref"), node, role)
		if role != "schema" && role != "path" {
			return
		}
	}
	if role == "example" && has(m, "externalValue") {
		g.addUse(m["externalValue"], g.childPath(path, "externalValue"), node, "externalValue")
	}
	if role == "link" && has(m, "operationRef") {
		g.addUse(m["operationRef"], g.childPath(path, "operationRef"), node, "operation")
	}
	if role == "discriminator" {
		mapping, _ := m["mapping"].(map[string]any)
		for _, key := range sortedKeys(mapping) {
			if g.budget.exceeded {
				return
			}
			g.addMapping(mapping[key], g.childPath(g.childPath(path, "mapping"), key), node)
		}
		if has(m, "defaultMapping") {
			g.addMapping(m["defaultMapping"], g.childPath(path, "defaultMapping"), node)
		}
	}
	for _, key := range sortedKeys(m) {
		if g.budget.exceeded {
			return
		}
		if strings.HasPrefix(key, "x-") {
			continue
		}
		child := childRole(role, key)
		if child != "" {
			g.collect(m[key], g.childPath(path, key), child, base, document, false)
		}
	}
}

// 同一 URI 只能识别一个资源；同一节点的检索地址与自声明地址可以相同。
func (g *referenceGraph) addResource(uri string, node *referenceNode) {
	if old := g.resources[uri]; old != nil && old.path != node.path {
		g.add("resource.duplicate", node.path, "资源 URI 与 "+old.path+" 重复："+uri)
		return
	}
	if !g.resourceNodes[node.path] {
		if len(g.resourceNodes) >= g.maxResources {
			if !g.resourceLimitReported {
				g.add("budget", node.path, "包含内嵌 $id 的资源数量超过预算")
				g.resourceLimitReported = true
			}
			return
		}
		g.resourceNodes[node.path] = true
	}
	g.resources[uri] = node
}

// 校验标准锚点的字符范围。
func validAnchor(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
		if !letter && (i == 0 || !(r >= '0' && r <= '9' || r == '-' || r == '.')) {
			return false
		}
	}
	return true
}

// 对引用次数设定硬上限，超限明确失败而不伪装为完整检查。
func (g *referenceGraph) addUse(value any, path string, node *referenceNode, role string) {
	if g.budget.exceeded {
		return
	}
	if len(g.uses) >= g.maxReferences {
		if !g.limitReported {
			g.add("budget", path, "引用数量超过预算")
			g.limitReported = true
		}
		return
	}
	ref, ok := value.(string)
	if !ok {
		g.add("ref.uri", path, "引用必须是 URI-reference 字符串")
		return
	}
	g.uses = append(g.uses, referenceUse{value: ref, path: path, base: node.base, role: role, owner: node.path})
}

// discriminator 的已知组件名按名称解析，其他字符串保留 URI 语义。
func (g *referenceGraph) addMapping(value any, path string, node *referenceNode) {
	if g.budget.exceeded {
		return
	}
	ref, ok := value.(string)
	if !ok {
		g.add("ref.uri", path, "映射必须是组件名或 URI-reference 字符串")
		return
	}
	// 组件名称是否存在由完整文档查询，不依赖遍历时机。
	doc := g.nodes[node.document]
	if doc != nil {
		root, _ := doc.value.(map[string]any)
		components, _ := root["components"].(map[string]any)
		schemas, _ := components["schemas"].(map[string]any)
		if _, exists := schemas[ref]; exists {
			copy := *node
			copy.base = doc.base
			target := "#/components/schemas/" + escape(ref)
			// 名称映射直接识别组件；组件有 $id 时使用该资源身份。
			schema, _ := schemas[ref].(map[string]any)
			if id, ok := schema["$id"].(string); ok {
				if resolved, err := absoluteReference(doc.base, id); err == nil && resolved.Fragment == "" {
					target = resourceURI(resolved)
				}
			}
			if !g.spend(len(target)) {
				return
			}
			g.addUse(target, path, &copy, "schema")
			return
		}
	}
	g.addUse(value, path, node, "schema")
}

// 检查所有引用的初始目标，不递归展开循环，也不执行实例验证。
func (g *referenceGraph) checkReferences() {
	for _, use := range g.uses {
		if g.budget.exceeded {
			return
		}
		_, code, message := g.target(use)
		if code != "" {
			g.add(code, use.path, message)
		}
	}
}

// 解析目标资源后严格检查 JSON Pointer 或锚点以及对象种类。
func (g *referenceGraph) target(use referenceUse) (*referenceNode, string, string) {
	if !g.spend(len(use.base), len(use.value)) {
		return nil, "budget", "引用解析超过索引预算"
	}
	u, err := absoluteReference(use.base, use.value)
	if err != nil {
		return nil, "ref.uri", "引用不是合法 URI-reference：" + use.value
	}
	uri := resourceURI(u)
	if !g.spend(len(uri)) {
		return nil, "budget", "引用解析超过索引预算"
	}
	resource := g.resources[uri]
	if use.role == "externalValue" {
		if resource != nil {
			return resource, "", ""
		}
		if g.exampleResources[uri] {
			return nil, "", ""
		}
	}
	if resource == nil {
		return nil, "external.denied", "未提供引用所需的离线资源：" + uri
	}
	var target *referenceNode
	if u.Fragment == "" {
		target = resource
	} else if strings.HasPrefix(u.Fragment, "/") {
		path, code := g.pointerPath(resource, u.Fragment)
		if code != "" {
			return nil, code, "JSON Pointer 无效或目标不存在：" + use.value
		}
		target = g.nodes[path]
		if target != nil && target.role == "schema" && target.base != resource.base {
			return nil, "ref.scope", "JSON Pointer 跨越了目标 Schema 的 $id；请使用最近的 $id 作为引用基准"
		}
		if target == nil {
			return nil, "ref.type", "引用指向数据或未知规范对象：" + use.value
		}
	} else {
		if !g.spend(len(resource.base), 1, len(u.Fragment)) {
			return nil, "budget", "锚点解析超过索引预算"
		}
		target = g.anchors[resource.base+"#"+u.Fragment]
	}
	if target == nil {
		return nil, "ref.missing", "引用目标不存在：" + use.value
	}
	if target.role != use.role {
		return nil, "ref.type", "引用目标需要 " + use.role + "，实际为 " + target.role
	}
	return target, "", ""
}

// 将 URI 解码后的 JSON Pointer 转换为物理路径，严格遵守转义与数组索引语法。
func pointerPath(resource *referenceNode, fragment string) (string, string) {
	cur := resource.value
	parts := []string{resource.path}
	for _, encoded := range strings.Split(fragment[1:], "/") {
		for i := 0; i < len(encoded); i++ {
			if encoded[i] == '~' {
				if i+1 == len(encoded) || encoded[i+1] != '0' && encoded[i+1] != '1' {
					return "", "ref.pointer"
				}
				i++
			}
		}
		part := strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
		switch object := cur.(type) {
		case map[string]any:
			var exists bool
			cur, exists = object[part]
			if !exists {
				return "", "ref.missing"
			}
		case []any:
			if part == "" || len(part) > 1 && part[0] == '0' {
				return "", "ref.pointer"
			}
			for _, r := range part {
				if r < '0' || r > '9' {
					return "", "ref.pointer"
				}
			}
			i, err := strconv.Atoi(part)
			if err != nil || i >= len(object) {
				return "", "ref.missing"
			}
			cur = object[i]
		default:
			return "", "ref.missing"
		}
		parts = append(parts, escape(part))
	}
	return strings.Join(parts, "/"), ""
}
