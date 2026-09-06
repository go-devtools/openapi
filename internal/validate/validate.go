// Validate OAS 3.2 structure and references offline without framework rules.
package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Store specification errors and remediation advice.
type Issue struct {
	Code    string
	Path    string
	Message string
	Fix     string
}

// Keep validation state private to each invocation.
type checker struct {
	root   map[string]any
	issues []Issue
	ids    map[string]string
	graph  *referenceGraph
}

// Bound input size, depth, and nodes while validating without network access.
func Check(raw []byte) []Issue {
	return CheckWithOptions(raw, Options{})
}

// Distinguish exhausted input budgets from ordinary JSON syntax errors.
var errJSONBudget = errors.New("JSON depth or node count exceeds the budget")

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
				return nil, fmt.Errorf("object key is not a string")
			}
			if _, ok := out[s]; ok {
				return nil, fmt.Errorf("duplicate object key: %s", s)
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
		return nil, fmt.Errorf("invalid JSON delimiter")
	}
}

// Collect namespaced diagnostics without hiding error locations.
func (c *checker) add(code, path, msg string) {
	if c.graph != nil && !c.graph.spend(len(code), len(path), len(msg), 128) {
		return
	}
	c.issues = append(c.issues, Issue{Code: "openapi.spec." + code, Path: path, Message: msg, Fix: "Fix the document construction or source contract; import external content explicitly as offline resources"})
}

// Escape a JSON Pointer segment.
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

// Read a string field; individual rules handle missing or invalid values.
func str(m map[string]any, k string) string { v, _ := m[k].(string); return v }

// Check whether a key is explicitly present.
func has(m map[string]any, k string) bool { _, ok := m[k]; return ok }

// Traverse standard object roles while treating examples and extensions as data.
func (c *checker) walk(v any, path, role string, depth int) {
	if c.stopped() {
		return
	}
	if depth > 128 {
		c.add("budget", path, "specification object recursion exceeds the budget")
		return
	}
	if role == "security" || role == "example" || role == "examples" || role == "discriminator" || role == "xml" {
		if _, ok := v.(map[string]any); !ok {
			c.add("object", path, role+" must be an object")
			return
		}
	}
	if role == "schemaArray" {
		items, ok := v.([]any)
		if !ok || len(items) == 0 {
			c.add("schema.keyword", path, "keyword requires an array containing at least one Schema")
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
		c.add("schema.keyword", path, "Schema or Schema dictionary cannot be an array")
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
		c.add("object", path, "object required here")
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
				c.add("response.status", path+"/"+escape(k), "invalid response status code")
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
				c.add("ref.sibling", path, "Reference Object contains an invalid sibling field: "+k)
			}
		}
		return
	}
	if (role == "root" || role == "operation") && has(m, "security") {
		requirements, ok := m["security"].([]any)
		if !ok {
			c.add("security.requirements", path+"/security", "security must be an array, not null")
		} else {
			for i, requirement := range requirements {
				if c.stopped() {
					return
				}
				entry, ok := requirement.(map[string]any)
				if !ok {
					c.add("security.requirements", path+"/security/"+strconv.Itoa(i), "each security requirement must be an object")
					continue
				}
				for _, name := range sortedKeys(entry) {
					scopes := entry[name]
					if c.stopped() {
						return
					}
					values, ok := scopes.([]any)
					if !ok {
						c.add("security.scopes", path+"/security/"+strconv.Itoa(i), name+" scopes must be an array")
						continue
					}
					for _, scope := range values {
						if c.stopped() {
							return
						}
						if _, ok := scope.(string); !ok {
							c.add("security.scopes", path+"/security/"+strconv.Itoa(i), "scope must be a string")
						}
					}
				}
			}
		}
	}
	switch role {
	case "root":
		if str(m, "openapi") != "3.2.0" {
			c.add("version", path, "native openapi 3.2.0 is required")
		}
		if _, ok := m["info"].(map[string]any); !ok {
			c.add("info", path, "info object is required")
		}

	case "info":
		if str(m, "title") == "" || str(m, "version") == "" {
			c.add("info", path, "title and version must not be empty")
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
					c.add("method.duplicate", path, "fixed HTTP methods cannot appear in additionalOperations")
				}
			}
		}
	case "operation":
		if id := str(m, "operationId"); id != "" {
			if old, ok := c.ids[id]; ok {
				c.add("operationId.duplicate", path, "operationId conflicts with "+old+" is duplicated")
			}
			c.ids[id] = path
		}
		if responses, ok := m["responses"].(map[string]any); !ok || len(responses) == 0 {
			c.add("response.missing", path, "operation has no response")
		}
		c.parameters(m, path)
	case "parameter":
		in := str(m, "in")
		if in != "path" && in != "query" && in != "header" && in != "cookie" && in != "querystring" {
			c.add("parameter.location", path, "invalid parameter location")
		}
		if str(m, "name") == "" {
			c.add("parameter.name", path, "named parameter requires name")
		}
		if in == "path" && m["required"] != true {
			c.add("parameter.required", path, "path parameter must be required")
		}
		if in == "querystring" && !has(m, "content") {
			c.add("parameter.querystring", path, "querystring must use content")
		}
		c.parameterContent(m, path)
	case "response":
		for _, field := range []string{"summary", "description"} {
			if value, exists := m[field]; exists {
				if _, ok := value.(string); !ok {
					c.add("response."+field, path, "Response "+field+" must be a string")
				}
			}
		}
	case "header":
		c.parameterContent(m, path)
	case "requestBody":
		if content, ok := m["content"].(map[string]any); !ok || len(content) == 0 {
			c.add("request.content", path, "request body requires a media type")
		}
	case "media", "encoding":
		if has(m, "encoding") && (has(m, "prefixEncoding") || has(m, "itemEncoding")) {
			c.add("encoding.conflict", path, "named and positional encoding cannot be combined")
		}
		if role == "media" && (has(m, "prefixEncoding") || has(m, "itemEncoding")) && !has(m, "itemSchema") {
			schema, _ := m["schema"].(map[string]any)
			if !schemaHasType(schema, "array") {
				c.add("encoding.array", path, "positional encoding requires an array schema or itemSchema")
			}
		}
		if has(m, "example") && has(m, "examples") {
			c.add("example.conflict", path, "example and examples are mutually exclusive")
		}
	case "example":
		c.example(m, path)
	case "tag":
		if str(m, "name") == "" {
			c.add("tag.name", path, "tag name must not be empty")
		}
	case "link":
		if has(m, "operationRef") == has(m, "operationId") {
			c.add("link.target", path, "link must select exactly one operation target")
		}
	case "security":
		c.security(m, path)
	case "discriminator":
		c.discriminator(m, path)
	case "xml":
		c.xml(m, path)
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
		return map[string]string{"schemas": "schemas", "responses": "responseComponents", "parameters": "parameters", "headers": "headers", "mediaTypes": "mediaTypes", "examples": "examples", "securitySchemes": "securitySchemes", "links": "links", "callbacks": "callbacks", "pathItems": "pathItems", "requestBodies": "requestBodies"}[k]
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

// Validate standard path braces without interpreting framework syntax.
func (c *checker) checkPath(path, parent string) {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#\r\n") {
		c.add("path", parent, "invalid OpenAPI path: "+path)
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
				c.add("path", parent, "nested path template")
			}
			inside = true
			start = i
		case '}':
			if !inside || i == start+1 {
				c.add("path", parent, "empty or mismatched path template")
			}
			inside = false
		}
	}
	if inside {
		c.add("path", parent, "unterminated path template")
	}
}

// Recognize the fixed OpenAPI methods.
func isFixed(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "QUERY":
		return true
	}
	return false
}

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

// Validate parameter schema/content and serialization constraints.
func (c *checker) parameterContent(m map[string]any, path string) {
	if has(m, "schema") == has(m, "content") {
		c.add("parameter.content", path, "parameter must use exactly one of schema or content")
	}
	if content, ok := m["content"].(map[string]any); ok {
		if len(content) != 1 {
			c.add("parameter.content", path, "parameter content must have exactly one media type")
		}
		for _, k := range []string{"style", "explode", "allowReserved"} {
			if c.stopped() {
				return
			}
			if has(m, k) {
				c.add("parameter.serialization", path, "content cannot be combined with "+k+" and ")
			}
		}
	}
	if has(m, "example") && has(m, "examples") {
		c.add("example.conflict", path, "parameter example and examples are mutually exclusive")
	}
}

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
			c.add("parameter.duplicate", path, "duplicate parameter "+key)
		}
		seen[key] = true
		query = query || in == "query"
		querystring = querystring || in == "querystring"
	}
	if query && querystring {
		c.add("parameter.querystring", path, "query and querystring cannot be combined")
	}
}

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
			c.add("tag.duplicate", "#/tags", "duplicate tag: "+name)
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
				c.add("tag.cycle", "#/tags", "tag parent cycle: "+name)
				break
			}
			seen[cur] = true
			next, ok := parents[cur]
			if !ok {
				c.add("tag.parent", "#/tags", "tag parent does not exist: "+cur)
				break
			}
			cur = next
		}
	}
}
