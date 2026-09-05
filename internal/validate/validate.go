// 离线检查 OpenAPI 三点二结构、约束与引用，不包含任何框架规则。
package validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// 保存规范错误及修复建议。
type Issue struct {
	Code    string
	Path    string
	Message string
	Fix     string
}

// 保存单次验证状态，实例间不共享可变数据。
type checker struct {
	root   map[string]any
	issues []Issue
	ids    map[string]string
	refs   map[string]string
	nodes  int
}

// 限制输入体积、深度、对象数量并执行无网络检查。
func Check(raw []byte) []Issue {
	c := checker{ids: map[string]string{}, refs: map[string]string{}}
	if len(raw) > 8<<20 {
		c.add("budget", "#", "文档超过八 MiB 限制")
		return c.issues
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	value, err := decode(dec, 0, new(int))
	if err != nil {
		c.add("json", "#", err.Error())
		return c.issues
	}
	if _, err = dec.Token(); err != io.EOF {
		c.add("json", "#", "文档末尾有额外内容")
		return c.issues
	}
	root, ok := value.(map[string]any)
	if !ok {
		c.add("root", "#", "文档根必须是对象")
		return c.issues
	}
	c.root = root
	c.walk(root, "#", "root", 0)
	c.checkTags()
	for ref, path := range c.refs {
		if strings.HasPrefix(ref, "#/") && !c.resolve(ref) {
			c.add("ref.missing", path, "引用目标不存在："+ref)
		}
	}
	sort.Slice(c.issues, func(i, j int) bool {
		if c.issues[i].Path != c.issues[j].Path {
			return c.issues[i].Path < c.issues[j].Path
		}
		return c.issues[i].Code < c.issues[j].Code
	})
	return c.issues
}

// 用 token 解码检测重复键，并保留所有数值的十进制文本。
func decode(dec *json.Decoder, depth int, count *int) (any, error) {
	*count++
	if depth > 128 || *count > 200000 {
		return nil, fmt.Errorf("JSON 深度或节点数超过预算")
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
func (c *checker) add(code, path, msg string) {
	c.issues = append(c.issues, Issue{Code: "openapi.spec." + code, Path: path, Message: msg, Fix: "修正规范构造或相应源码契约；外部内容请显式离线导入"})
}

// 以 JSON Pointer 编码路径片段。
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

// 获取字符串字段；缺失和类型错误由具体规则区分。
func str(m map[string]any, k string) string { v, _ := m[k].(string); return v }

// 检查键是否明确存在。
func has(m map[string]any, k string) bool { _, ok := m[k]; return ok }

// 按标准对象上下文遍历，示例值和扩展值作为数据而不是规范关键字。
func (c *checker) walk(v any, path, role string, depth int) {
	if depth > 128 {
		c.add("budget", path, "规范对象递归超过预算")
		return
	}
	if a, ok := v.([]any); ok {
		for i, item := range a {
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
	switch role {
	case "schemas", "paths", "responses", "parameters", "headers", "mediaTypes", "examples", "securitySchemes", "links", "callbacks", "pathItems", "encodings", "webhooks":
		singular := map[string]string{"schemas": "schema", "paths": "path", "responses": "response", "parameters": "parameter", "headers": "header", "mediaTypes": "media", "examples": "example", "securitySchemes": "security", "links": "link", "callbacks": "callback", "pathItems": "path", "encodings": "encoding", "webhooks": "path"}[role]
		for k, item := range m {
			if strings.HasPrefix(k, "x-") {
				continue
			}
			if role == "paths" {
				c.checkPath(k, path)
			}
			if role == "responses" && !validStatus(k) {
				c.add("response.status", path+"/"+k, "非法响应状态码")
			}
			c.walk(item, path+"/"+escape(k), singular, depth+1)
		}
		return
	}
	if ref := str(m, "$ref"); ref != "" {
		c.reference(ref, path)
		if role != "schema" && role != "path" {
			for k := range m {
				if k != "$ref" && k != "summary" && k != "description" {
					c.add("ref.sibling", path, "Reference Object 存在非法兄弟字段："+k)
				}
			}
			return
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
		if self := str(m, "$self"); self != "" {
			u, err := url.Parse(self)
			if err != nil || !u.IsAbs() || u.Fragment != "" {
				c.add("self", path, "$self 必须是无片段的绝对 URI")
			}
		}
	case "info":
		if str(m, "title") == "" || str(m, "version") == "" {
			c.add("info", path, "title 和 version 不能为空")
		}
	case "schema":
		c.schema(m, path)
	case "path":
		if extra, ok := m["additionalOperations"].(map[string]any); ok {
			for method := range extra {
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
		if in != "querystring" && str(m, "name") == "" {
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
		if has(m, "externalValue") {
			c.add("external.denied", path, "默认禁止外部示例加载")
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
		if has(m, "defaultMapping") && !has(m, "mapping") { /* 默认映射可以单独表达，不推断业务分派。 */
		}
	case "xml":
		if node := str(m, "nodeType"); node != "" && node != "element" && node != "attribute" && node != "text" && node != "cdata" && node != "none" {
			c.add("xml.nodeType", path, "未知 XML 节点类型")
		}
	}
	for k, item := range m {
		if strings.HasPrefix(k, "x-") {
			continue
		}
		child := c.childRole(role, k)
		if child != "" {
			c.walk(item, path+"/"+escape(k), child, depth+1)
		}
	}
}

// 返回已知结构字段的子对象上下文。
func (c *checker) childRole(role, k string) string {
	if role == "schema" {
		switch k {
		case "properties", "patternProperties", "$defs", "dependentSchemas":
			return "schemas"
		case "items", "prefixItems", "contains", "unevaluatedItems", "additionalProperties", "unevaluatedProperties", "propertyNames", "allOf", "anyOf", "oneOf", "not", "if", "then", "else", "contentSchema":
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
	switch k {
	case "info":
		return "info"
	case "paths":
		return "paths"
	case "webhooks":
		return "webhooks"
	case "components":
		return "components"
	case "additionalOperations":
		return "additionalOperations"
	case "responses":
		return "responses"
	case "parameters":
		return "parameter"
	case "requestBody":
		return "requestBody"
	case "headers":
		return "headers"
	case "content":
		return "mediaTypes"
	case "schema", "itemSchema":
		return "schema"
	case "encoding":
		return "encodings"
	case "prefixEncoding", "itemEncoding":
		return "encoding"
	case "examples":
		return "examples"
	case "links":
		return "links"
	case "callbacks":
		return "callbacks"
	case "tags":
		if role == "root" {
			return "tag"
		}
	}
	return ""
}

// 检查中立路径花括号结构，不解释框架专用语法。
func (c *checker) checkPath(path, parent string) {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#\r\n") {
		c.add("path", parent, "非法 OpenAPI 路径："+path)
	}
	inside := false
	start := 0
	for i, r := range path {
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
func isFixed(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "QUERY":
		return true
	}
	return false
}

// 接受具体状态码、规范范围和显式 default。
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
func (c *checker) parameterContent(m map[string]any, path string) {
	if has(m, "schema") == has(m, "content") {
		c.add("parameter.content", path, "参数必须且只能使用 schema 或 content")
	}
	if content, ok := m["content"].(map[string]any); ok {
		if len(content) != 1 {
			c.add("parameter.content", path, "参数 content 必须且只能有一个媒体类型")
		}
		for _, k := range []string{"style", "explode", "allowReserved"} {
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
func (c *checker) parameters(m map[string]any, path string) {
	list, _ := m["parameters"].([]any)
	seen := map[string]bool{}
	query, querystring := false, false
	for _, v := range list {
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

// 检查可确定的 Schema 约束矛盾，不从自然语言推断限制。
func (c *checker) schema(m map[string]any, path string) {
	if has(m, "nullable") {
		c.add("schema.nullable", path, "应使用联合 null，不能输出旧版 nullable")
	}
	if ref := str(m, "$dynamicRef"); ref != "" {
		c.reference(ref, path)
	}
	if has(m, "type") {
		for _, entry := range []struct {
			keys  []string
			types []string
		}{{[]string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf"}, []string{"number", "integer"}}, {[]string{"minLength", "maxLength", "pattern"}, []string{"string"}}, {[]string{"minItems", "maxItems", "uniqueItems", "contains"}, []string{"array"}}} {
			applies := false
			for _, t := range entry.types {
				applies = applies || schemaHasType(m, t)
			}
			if !applies {
				for _, k := range entry.keys {
					if has(m, k) {
						c.add("schema.type", path, k+" 不适用于显式类型")
					}
				}
			}
		}
	}
	for _, pair := range [][2]string{{"minimum", "maximum"}, {"minLength", "maxLength"}, {"minItems", "maxItems"}, {"minContains", "maxContains"}, {"minProperties", "maxProperties"}} {
		a, aok := m[pair[0]].(json.Number)
		b, bok := m[pair[1]].(json.Number)
		if aok && bok {
			ar, ok1 := new(big.Rat).SetString(string(a))
			br, ok2 := new(big.Rat).SetString(string(b))
			if ok1 && ok2 && ar.Cmp(br) > 0 {
				c.add("schema.range", path, pair[0]+" 大于 "+pair[1])
			}
		}
	}
}

// 记录引用但不下载；本地 JSON Pointer 在完整遍历后检查。
func (c *checker) reference(ref, path string) {
	if !strings.HasPrefix(ref, "#") {
		c.add("external.denied", path, "默认禁止外部引用："+ref)
		return
	}
	c.refs[ref] = path
}

// 解析本地 JSON Pointer，不进行文件或网络访问。
func (c *checker) resolve(ref string) bool {
	fragment, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
	if err != nil {
		return false
	}
	var cur any = c.root
	for _, part := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch x := cur.(type) {
		case map[string]any:
			var ok bool
			cur, ok = x[part]
			if !ok {
				return false
			}
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(x) {
				return false
			}
			cur = x[i]
		default:
			return false
		}
	}
	return true
}

// 验证标签引用存在并检查父级环。
func (c *checker) checkTags() {
	tags, _ := c.root["tags"].([]any)
	parents := map[string]string{}
	for _, v := range tags {
		m, _ := v.(map[string]any)
		name := str(m, "name")
		if _, ok := parents[name]; ok {
			c.add("tag.duplicate", "#/tags", "标签重复："+name)
		}
		parents[name] = str(m, "parent")
	}
	for name := range parents {
		seen := map[string]bool{}
		cur := name
		for cur != "" {
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
		for kind, v := range flows {
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
