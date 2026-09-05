// 离线检查 OpenAPI 三点二结构、约束与引用，不包含任何框架规则。
// Validate OAS 3.2 structure and references offline without framework rules.
package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 保存规范错误及修复建议。
// Store specification errors and remediation advice.
type Issue struct {
	Code    string
	Path    string
	Message string
	Fix     string
}

// 保存单次验证状态，实例间不共享可变数据。
// Keep validation state private to each invocation.
type checker struct {
	root   map[string]any
	issues []Issue
	ids    map[string]string
	graph  *referenceGraph
}

// 限制输入体积、深度、对象数量并执行无网络检查。
// Bound input size, depth, and nodes while validating without network access.
func Check(raw []byte) []Issue {
	return CheckWithOptions(raw, Options{})
}

// 区分输入预算耗尽与普通 JSON 语法错误。
// Distinguish exhausted input budgets from ordinary JSON syntax errors.
var errJSONBudget = errors.New("JSON 深度或节点数超过预算")

// 用 token 解码检测重复键，并保留所有数值的十进制文本。
// Decode tokens to detect duplicate keys and preserve decimal number text.
func decode(dec *json.Decoder, depth int, count *int) (any, error) {
	*count++
	if depth > 128 || *count > 200000 {
		return nil, errJSONBudget
	}
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		out := map[string]any{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			s, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("对象键不是字符串")
			}
			if _, ok := out[s]; ok {
				return nil, fmt.Errorf("重复对象键：%s", s)
			}
			v, err := decode(dec, depth+1, count)
			if err != nil {
				return nil, err
			}
			out[s] = v
		}
		_, err = dec.Token()
		return out, err
	case '[':
		out := []any{}
		for dec.More() {
			v, err := decode(dec, depth+1, count)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		_, err = dec.Token()
		return out, err
	default:
		return nil, fmt.Errorf("错误的 JSON 分隔符")
	}
}

// 聚合稳定命名空间的错误，不隐藏失败位置。
// Collect namespaced diagnostics without hiding error locations.
func (c *checker) add(code, path, msg string) {
	if c.graph != nil && !c.graph.spend(len(code), len(path), len(msg), 128) {
		return
	}
	c.issues = append(c.issues, Issue{Code: "openapi.spec." + code, Path: path, Message: msg, Fix: "修正规范构造或相应源码契约；外部内容请显式离线导入"})
}

// 以 JSON Pointer 编码路径片段。
// Escape a JSON Pointer segment.
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

// 获取字符串字段；缺失和类型错误由具体规则区分。
// Read a string field; individual rules handle missing or invalid values.
func str(m map[string]any, k string) string { v, _ := m[k].(string); return v }

// 检查键是否明确存在。
// Check whether a key is explicitly present.
func has(m map[string]any, k string) bool { _, ok := m[k]; return ok }

// 按标准对象上下文遍历，示例值和扩展值作为数据而不是规范关键字。
// Traverse standard object roles while treating examples and extensions as data.
func (c *checker) walk(v any, path, role string, depth int) {
	if c.stopped() {
		return
	}
	if depth > 128 {
		c.add("budget", path, "规范对象递归超过预算")
		return
	}
	if role == "schemaArray" {
		items, ok := v.([]any)
		if !ok || len(items) == 0 {
			c.add("schema.keyword", path, "此关键字要求至少一个 Schema 的数组")
			return
		}
		for i, item := range items {
			if c.stopped() {
				return
			}
			c.walk(item, path+"/"+strconv.Itoa(i), "schema", depth+1)
		}
		return
	}
	if _, ok := v.([]any); ok && (role == "schema" || role == "schemas") {
		c.add("schema.keyword", path, "Schema 或 Schema 字典不能是数组")
		return
	}
	if a, ok := v.([]any); ok {
		for i, item := range a {
			if c.stopped() {
				return
			}
			c.walk(item, path+"/"+strconv.Itoa(i), role, depth+1)
		}
		return
	}
	if role == "schema" {
		if _, ok := v.(bool); ok {
			return
		}
	}
	m, ok := v.(map[string]any)
	if !ok {
		c.add("object", path, "此处必须是对象")
		return
	}
	if singular := dictionaryRole(role); singular != "" {
		for _, k := range sortedKeys(m) {
			if c.stopped() {
				return
			}
			if dictionaryExtension(role, k) {
				continue
			}
			if role == "paths" {
				c.checkPath(k, path)
			}
			if role == "responses" && !validStatus(k) {
				c.add("response.status", path+"/"+escape(k), "非法响应状态码")
			}
			c.walk(m[k], path+"/"+escape(k), singular, depth+1)
		}
		return
	}
	if has(m, "$ref") && referenceRole(role) && role != "schema" && role != "path" {
		for _, k := range sortedKeys(m) {
			if c.stopped() {
				return
			}
			if k != "$ref" && k != "summary" && k != "description" {
				c.add("ref.sibling", path, "Reference Object 存在非法兄弟字段："+k)
			}
		}
		return
	}
	if (role == "root" || role == "operation") && has(m, "security") {
		requirements, ok := m["security"].([]any)
		if !ok {
			c.add("security.requirements", path+"/security", "security 必须是数组，不能是 null")
		} else {
			for i, requirement := range requirements {
				if c.stopped() {
					return
				}
				entry, ok := requirement.(map[string]any)
				if !ok {
					c.add("security.requirements", path+"/security/"+strconv.Itoa(i), "每项安全要求必须是对象")
					continue
				}
				for _, name := range sortedKeys(entry) {
					scopes := entry[name]
					if c.stopped() {
						return
					}
					values, ok := scopes.([]any)
					if !ok {
						c.add("security.scopes", path+"/security/"+strconv.Itoa(i), name+" 的作用域必须是数组")
						continue
					}
					for _, scope := range values {
						if c.stopped() {
							return
						}
						if _, ok := scope.(string); !ok {
							c.add("security.scopes", path+"/security/"+strconv.Itoa(i), "作用域必须是字符串")
						}
					}
				}
			}
		}
	}
	switch role {
	case "root":
		if str(m, "openapi") != "3.2.0" {
			c.add("version", path, "需要原生 openapi 3.2.0")
		}
		if _, ok := m["info"].(map[string]any); !ok {
			c.add("info", path, "缺少 info 对象")
		}

	case "info":
		if str(m, "title") == "" || str(m, "version") == "" {
			c.add("info", path, "title 和 version 不能为空")
		}
	case "schema":
		c.schema(m, path)
	case "path":
		if extra, ok := m["additionalOperations"].(map[string]any); ok {
			for _, method := range sortedKeys(extra) {
				if c.stopped() {
					return
				}
				if isFixed(strings.ToUpper(method)) {
					c.add("method.duplicate", path, "固定 HTTP 方法不能出现在 additionalOperations")
				}
			}
		}
	case "operation":
		if id := str(m, "operationId"); id != "" {
			if old, ok := c.ids[id]; ok {
				c.add("operationId.duplicate", path, "operationId 与 "+old+" 重复")
			}
			c.ids[id] = path
		}
		if responses, ok := m["responses"].(map[string]any); !ok || len(responses) == 0 {
			c.add("response.missing", path, "接口缺少响应")
		}
		c.parameters(m, path)
	case "parameter":
		in := str(m, "in")
		if in != "path" && in != "query" && in != "header" && in != "cookie" && in != "querystring" {
			c.add("parameter.location", path, "参数位置不合法")
		}
		if str(m, "name") == "" {
			c.add("parameter.name", path, "命名参数缺少 name")
		}
		if in == "path" && m["required"] != true {
			c.add("parameter.required", path, "路径参数必须 required")
		}
		if in == "querystring" && !has(m, "content") {
			c.add("parameter.querystring", path, "querystring 必须使用 content")
		}
		c.parameterContent(m, path)
	case "header":
		c.parameterContent(m, path)
	case "requestBody":
		if content, ok := m["content"].(map[string]any); !ok || len(content) == 0 {
			c.add("request.content", path, "请求体缺少媒体类型")
		}
	case "media", "encoding":
		if has(m, "encoding") && (has(m, "prefixEncoding") || has(m, "itemEncoding")) {
			c.add("encoding.conflict", path, "命名编码不能与位置编码并用")
		}
		if role == "media" && (has(m, "prefixEncoding") || has(m, "itemEncoding")) && !has(m, "itemSchema") {
			schema, _ := m["schema"].(map[string]any)
			if !schemaHasType(schema, "array") {
				c.add("encoding.array", path, "位置编码要求数组 schema 或 itemSchema")
			}
		}
		if has(m, "example") && has(m, "examples") {
			c.add("example.conflict", path, "example 与 examples 互斥")
		}
	case "example":
		if has(m, "value") && (has(m, "dataValue") || has(m, "serializedValue") || has(m, "externalValue")) {
			c.add("example.conflict", path, "value 与三点二示例表达分支互斥")
		}
		if has(m, "serializedValue") && has(m, "externalValue") {
			c.add("example.conflict", path, "serializedValue 与 externalValue 互斥")
		}
	case "tag":
		if str(m, "name") == "" {
			c.add("tag.name", path, "标签名称不能为空")
		}
	case "link":
		if has(m, "operationRef") == has(m, "operationId") {
			c.add("link.target", path, "链接必须且只能选择一个操作目标")
		}
	case "security":
		c.security(m, path)
	case "discriminator":
		if has(m, "defaultMapping") && !has(m, "mapping") { /* 默认映射可以单独表达，不推断业务分派。 A default mapping can stand alone without inferring business dispatch. */
		}
	case "xml":
		if node := str(m, "nodeType"); node != "" && node != "element" && node != "attribute" && node != "text" && node != "cdata" && node != "none" {
			c.add("xml.nodeType", path, "未知 XML 节点类型")
		}
	}
	for _, k := range sortedKeys(m) {
		if c.stopped() {
			return
		}
		item := m[k]
		if strings.HasPrefix(k, "x-") {
			continue
		}
		if c.graph != nil && c.graph.schemaOnly && role == "schema" && (k == "discriminator" || k == "xml") {
			continue
		}
		child := childRole(role, k)
		if child != "" {
			c.walk(item, path+"/"+escape(k), child, depth+1)
		}
	}
}

// 返回已知结构字段的子对象上下文。
// Select the child role for a recognized structural field.
func childRole(role, k string) string {
	if role == "schema" {
		switch k {
		case "properties", "patternProperties", "$defs", "dependentSchemas":
			return "schemas"
		case "prefixItems", "allOf", "anyOf", "oneOf":
			return "schemaArray"
		case "items", "contains", "unevaluatedItems", "additionalProperties", "unevaluatedProperties", "propertyNames", "not", "if", "then", "else", "contentSchema":
			return "schema"
		case "discriminator":
			return "discriminator"
		case "xml":
			return "xml"
		}
		return ""
	}
	if role == "components" {
		return map[string]string{"schemas": "schemas", "responses": "responses", "parameters": "parameters", "headers": "headers", "mediaTypes": "mediaTypes", "examples": "examples", "securitySchemes": "securitySchemes", "links": "links", "callbacks": "callbacks", "pathItems": "pathItems", "requestBodies": "requestBodies"}[k]
	}
	if role == "requestBodies" {
		return "requestBody"
	}
	if role == "additionalOperations" {
		return "operation"
	}
	if role == "callback" {
		return "path"
	}
	if role == "path" && isFixed(strings.ToUpper(k)) {
		return "operation"
	}
	// 相同字段名在 Link 等对象中可能是业务数据，不能按名字跨上下文解释。
	// Interpret fields within their object context; matching names inside Links may be business data.
	children := map[string]map[string]string{
		"root":        {"info": "info", "paths": "paths", "webhooks": "webhooks", "components": "components", "tags": "tag", "servers": "server"},
		"info":        {"license": "license", "contact": "contact"},
		"path":        {"parameters": "parameter", "additionalOperations": "additionalOperations", "servers": "server"},
		"operation":   {"parameters": "parameter", "responses": "responses", "requestBody": "requestBody", "callbacks": "callbacks", "servers": "server"},
		"requestBody": {"content": "mediaTypes"},
		"response":    {"headers": "headers", "content": "mediaTypes", "links": "links"},
		"parameter":   {"schema": "schema", "content": "mediaTypes", "examples": "examples"},
		"header":      {"schema": "schema", "content": "mediaTypes", "examples": "examples"},
		"media":       {"schema": "schema", "itemSchema": "schema", "examples": "examples", "encoding": "encodings", "prefixEncoding": "encoding", "itemEncoding": "encoding"},
		"encoding":    {"headers": "headers", "encoding": "encodings", "prefixEncoding": "encoding", "itemEncoding": "encoding"},
		"link":        {"server": "server"},
	}
	return children[role][k]
}

// 检查中立路径花括号结构，不解释框架专用语法。
// Validate standard path braces without interpreting framework syntax.
func (c *checker) checkPath(path, parent string) {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#\r\n") {
		c.add("path", parent, "非法 OpenAPI 路径："+path)
	}
	inside := false
	start := 0
	for i, r := range path {
		if c.stopped() {
			return
		}
		switch r {
		case '{':
			if inside {
				c.add("path", parent, "嵌套路径模板")
			}
			inside = true
			start = i
		case '}':
			if !inside || i == start+1 {
				c.add("path", parent, "空或不匹配的路径模板")
			}
			inside = false
		}
	}
	if inside {
		c.add("path", parent, "未闭合路径模板")
	}
}

// 判断 OpenAPI 已定义的固定方法。
// Recognize the fixed OpenAPI methods.
func isFixed(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "QUERY":
		return true
	}
	return false
}

// 接受具体状态码、规范范围和显式 default。
// Accept concrete status codes, status ranges, and explicit default responses.
func validStatus(status string) bool {
	if status == "default" {
		return true
	}
	if len(status) != 3 || status[0] < '1' || status[0] > '5' {
		return false
	}
	if status[1:] == "XX" {
		return true
	}
	_, err := strconv.Atoi(status)
	return err == nil
}

// 验证参数 schema/content 与序列化限制。
// Validate parameter schema/content and serialization constraints.
func (c *checker) parameterContent(m map[string]any, path string) {
	if has(m, "schema") == has(m, "content") {
		c.add("parameter.content", path, "参数必须且只能使用 schema 或 content")
	}
	if content, ok := m["content"].(map[string]any); ok {
		if len(content) != 1 {
			c.add("parameter.content", path, "参数 content 必须且只能有一个媒体类型")
		}
		for _, k := range []string{"style", "explode", "allowReserved"} {
			if c.stopped() {
				return
			}
			if has(m, k) {
				c.add("parameter.serialization", path, "content 不与 "+k+" 并用")
			}
		}
	}
	if has(m, "example") && has(m, "examples") {
		c.add("example.conflict", path, "参数 example 与 examples 互斥")
	}
}

// 检查同一接口的重复参数以及 querystring 与 query 冲突。
// Check duplicate parameters and querystring/query conflicts.
func (c *checker) parameters(m map[string]any, path string) {
	list, _ := m["parameters"].([]any)
	seen := map[string]bool{}
	query, querystring := false, false
	for _, v := range list {
		if c.stopped() {
			return
		}
		p, _ := v.(map[string]any)
		if has(p, "$ref") {
			continue
		}
		in := str(p, "in")
		key := in + ":" + str(p, "name")
		if seen[key] {
			c.add("parameter.duplicate", path, "重复参数 "+key)
		}
		seen[key] = true
		query = query || in == "query"
		querystring = querystring || in == "querystring"
	}
	if query && querystring {
		c.add("parameter.querystring", path, "query 与 querystring 不能并用")
	}
}

// 判断显式类型约束是否包含指定类型。
// Check whether a type constraint contains a specified type.
func schemaHasType(m map[string]any, want string) bool {
	if str(m, "type") == want {
		return true
	}
	if a, ok := m["type"].([]any); ok {
		for _, v := range a {
			if v == want {
				return true
			}
		}
	}
	return false
}

// 验证标签引用存在并检查父级环。
// Validate tag parents and detect hierarchy cycles.
func (c *checker) checkTags() {
	tags, _ := c.root["tags"].([]any)
	parents := map[string]string{}
	for _, v := range tags {
		if c.stopped() {
			return
		}
		m, _ := v.(map[string]any)
		name := str(m, "name")
		if _, ok := parents[name]; ok {
			c.add("tag.duplicate", "#/tags", "标签重复："+name)
		}
		parents[name] = str(m, "parent")
	}
	for _, name := range sortedKeys(parents) {
		if c.stopped() {
			return
		}
		seen := map[string]bool{}
		cur := name
		for cur != "" {
			if c.stopped() {
				return
			}
			if c.graph != nil && !c.graph.spend(len(cur), 1) {
				return
			}
			if seen[cur] {
				c.add("tag.cycle", "#/tags", "标签父级成环："+name)
				break
			}
			seen[cur] = true
			next, ok := parents[cur]
			if !ok {
				c.add("tag.parent", "#/tags", "标签父级不存在："+cur)
				break
			}
			cur = next
		}
	}
}

// 校验安全方案的必需字段及设备授权端点。
// Check required security fields and device authorization endpoints.
func (c *checker) security(m map[string]any, path string) {
	switch str(m, "type") {
	case "apiKey":
		if str(m, "name") == "" || (str(m, "in") != "header" && str(m, "in") != "query" && str(m, "in") != "cookie") {
			c.add("security.apiKey", path, "apiKey 缺少合法 name/in")
		}
	case "http":
		if str(m, "scheme") == "" {
			c.add("security.http", path, "HTTP 认证缺少 scheme")
		}
	case "oauth2":
		flows, _ := m["flows"].(map[string]any)
		if len(flows) == 0 && !has(m, "oauth2MetadataUrl") {
			c.add("security.oauth2", path, "OAuth2 缺少流或元数据地址")
		}
		for _, kind := range sortedKeys(flows) {
			v := flows[kind]
			if c.stopped() {
				return
			}
			f, _ := v.(map[string]any)
			if kind == "deviceAuthorization" && (str(f, "deviceAuthorizationUrl") == "" || str(f, "tokenUrl") == "") {
				c.add("security.device", path, "设备授权缺少 deviceAuthorizationUrl 或 tokenUrl")
			}
			if !has(f, "scopes") {
				c.add("security.scopes", path, "OAuth 流缺少 scopes")
			}
		}
	case "openIdConnect":
		if str(m, "openIdConnectUrl") == "" {
			c.add("security.openId", path, "缺少发现地址")
		}
	case "mutualTLS":
	default:
		c.add("security.type", path, "未知安全方案类型")
	}
}
