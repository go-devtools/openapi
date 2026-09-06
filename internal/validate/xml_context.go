package validate

import (
	"mime"
	"strconv"
	"strings"
)

// Track reachable XML schemas without assigning application codec behavior to metadata.
type xmlContext struct {
	checker *checker
	queue   []*referenceNode
	seen    map[string]bool
}

// Start XML naming checks at content media types, retaining their context through media references.
func (c *checker) checkXMLContexts() {
	g := c.graph
	if g == nil || g.schemaOnly || c.stopped() {
		return
	}
	x := xmlContext{checker: c, seen: map[string]bool{}}
	for _, path := range sortedKeys(g.nodes) {
		if c.stopped() {
			return
		}
		node := g.nodes[path]
		if node.role != "media" {
			continue
		}
		parent, key := xmlPathParent(path)
		owner, field := xmlPathParent(parent)
		container := g.nodes[owner]
		if field != "content" || container == nil {
			continue
		}
		switch container.role {
		case "requestBody", "response", "parameter", "header":
			media, _, err := mime.ParseMediaType(strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~"))
			if err == nil && (media == "application/xml" || media == "text/xml" || strings.HasSuffix(media, "+xml")) {
				x.enqueue(node)
			}
		}
	}
	for i := 0; i < len(x.queue) && !c.stopped(); i++ {
		x.visit(x.queue[i])
	}
}

// Charge retained traversal state before scheduling each physical node once, including cycles.
func (x *xmlContext) enqueue(node *referenceNode) {
	if node != nil && !x.seen[node.path] && x.checker.graph.spend(64) {
		x.seen[node.path] = true
		x.queue = append(x.queue, node)
	}
}

// Reuse indexed schema paths rather than interpreting examples, extensions, or arbitrary JSON data.
func (x *xmlContext) child(node *referenceNode, keyword string) {
	g := x.checker.graph
	x.enqueue(g.nodes[g.childPath(node.path, keyword)])
}

// Follow ordinary references and positive schema structure; dynamic scope remains an instance concern.
func (x *xmlContext) visit(node *referenceNode) {
	c, g := x.checker, x.checker.graph
	object, ok := node.value.(map[string]any)
	if !ok || (node.role != "media" && node.role != "schema") {
		return
	}
	if value, ok := object["$ref"].(string); ok {
		target, code, _ := g.target(referenceUse{value: value, path: node.path + "/$ref", base: node.base, role: node.role, owner: node.path})
		if code == "" {
			x.enqueue(target)
		}
		if node.role == "media" {
			return // Reference Object siblings cannot introduce a second media schema.
		}
	}
	if node.role == "media" {
		x.child(node, "schema")
		x.child(node, "itemSchema")
		return
	}
	xml, _ := object["xml"].(map[string]any)
	kind := xmlNodeKind(object, xml)
	if (kind == "element" || kind == "attribute") && !has(xml, "name") && !x.inferredName(node) {
		c.add("xml.name.required", node.path+"/xml/name", "XML "+kind+" has no inferred name at this location; provide xml.name or use a named component/property schema")
	}
	for _, keyword := range []string{"properties", "dependentSchemas"} {
		entries, _ := object[keyword].(map[string]any)
		for _, key := range sortedKeys(entries) {
			if c.stopped() {
				return
			}
			x.enqueue(g.nodes[g.childPath(g.childPath(node.path, keyword), key)])
		}
	}
	for _, keyword := range []string{"prefixItems", "allOf", "anyOf", "oneOf"} {
		entries, _ := object[keyword].([]any)
		for i := range entries {
			if c.stopped() {
				return
			}
			x.enqueue(g.nodes[g.childPath(g.childPath(node.path, keyword), strconv.Itoa(i))])
		}
	}
	for _, keyword := range []string{"items", "then", "else"} {
		if has(object, keyword) && (keyword == "items" || has(object, "if")) {
			x.child(node, keyword)
		}
	}
}

// Apply native defaults and the two legacy true values without rewriting the supplied document.
func xmlNodeKind(schema, xml map[string]any) string {
	if has(xml, "nodeType") {
		return str(xml, "nodeType")
	}
	if xml["attribute"] == true {
		return "attribute"
	}
	if xml["wrapped"] == true && schemaHasType(schema, "array") {
		return "element"
	}
	if has(schema, "$ref") || has(schema, "$dynamicRef") || schemaHasType(schema, "array") {
		return "none"
	}
	return "element"
}

// Infer only physical component/property names; array items inherit a property name, never a wrapper name.
func (x *xmlContext) inferredName(node *referenceNode) bool {
	g := x.checker.graph
	component := true
	for node != nil && g.spend(32) {
		if component {
			prefix := node.document + "/components/schemas/"
			if strings.HasPrefix(node.path, prefix) && !strings.Contains(strings.TrimPrefix(node.path, prefix), "/") {
				return true
			}
		}
		parent, field := xmlPathParent(node.path)
		owner, container := xmlPathParent(parent)
		if container == "properties" && g.nodes[owner] != nil && g.nodes[owner].role == "schema" {
			return true
		}
		if container == "prefixItems" {
			parent = owner
		} else if field != "items" {
			return false
		}
		node = g.nodes[parent]
		if node == nil || node.role != "schema" {
			return false
		}
		object, _ := node.value.(map[string]any)
		if !schemaHasType(object, "array") {
			return false
		}
		component = false
	}
	return false
}

// Split one physical pointer segment without decoding escaped property names into separators.
func xmlPathParent(path string) (string, string) {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[:i], path[i+1:]
	}
	return "", path
}
