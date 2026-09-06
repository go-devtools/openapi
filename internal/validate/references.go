package validate

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Record a specification node's physical location, context, and owning document.
type referenceNode struct {
	value                      any
	path, role, base, document string
}

// Record every reference occurrence instead of deduplicating by reference text.
type referenceUse struct {
	value, path, base, role, owner string
}

// Index supplied content as an offline resource graph without file or network loaders.
type referenceGraph struct {
	schemaOnly            bool
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

// Build a bounded reference graph whose resource identities exist only for the current check.
func newReferenceGraph() *referenceGraph {
	return &referenceGraph{budget: byteBudget{remaining: 16 << 20}, maxNormalizedBytes: 16 << 20, nodes: map[string]*referenceNode{}, resources: map[string]*referenceNode{}, anchors: map[string]*referenceNode{}, maxReferences: 10000, maxResources: 64, resourceNodes: map[string]bool{}, exampleResources: map[string]bool{}}
}

// Record the same error codes used by ordinary checks.
func (g *referenceGraph) add(code, path, message string) {
	if !g.spend(len(code), len(path), len(message), 128) {
		return
	}
	g.issues = append(g.issues, Issue{Code: "openapi.spec." + code, Path: path, Message: message, Fix: "Fix the reference or explicitly provide offline resources; the checker does not read files or access the network"})
}

// Reject unescaped invalid URI characters while preserving valid relative references.
func parseURIReference(value string) (*url.URL, error) {
	for _, c := range value {
		if c <= 0x20 || c >= 0x7f || strings.ContainsRune("<>\"{}|\\^`", c) {
			return nil, fmt.Errorf("URI contains unencoded characters")
		}
	}
	return url.Parse(value)
}

// Resolve relative references into absolute URIs without performing I/O.
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

// Exclude fragments from resource identities; empty and absent fragments share one key.
func resourceURI(u *url.URL) string {
	copy := *u
	copy.Fragment, copy.RawFragment = "", ""
	copy.Scheme, copy.Host = strings.ToLower(copy.Scheme), strings.ToLower(copy.Host)
	return copy.String()
}

// Keeps traversal deterministic so duplicate-identity diagnostics are reproducible.
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Assign contexts to specification map entries; map keys are not themselves keywords.
func dictionaryRole(role string) string {
	switch role {
	case "schemas":
		return "schema"
	case "paths", "pathItems", "webhooks":
		return "path"
	case "responses", "responseComponents":
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

// Interpret x- keys as extensions only in extensible maps; always preserve schema property names.
func dictionaryExtension(role, key string) bool {
	return (role == "paths" || role == "responses") && strings.HasPrefix(key, "x-")
}

// Identify standard locations allowing Reference Objects or schema references.
func referenceRole(role string) bool {
	switch role {
	case "schema", "path", "response", "parameter", "header", "requestBody", "media", "example", "security", "link", "callback":
		return true
	}
	return false
}

// Index URIs, anchors, and specification objects without treating examples or extensions as reference instructions.
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
			g.add("self", g.childPath(path, "$self"), "$self must be a valid URI-reference")
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
			g.add("schema.id", g.childPath(path, "$id"), "$id must be a URI-reference without a nonempty fragment")
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
				g.add("schema.anchor", g.childPath(path, key), "anchor must start with a letter or underscore and contain only letters, digits, dots, hyphens, or underscores")
				continue
			}
			if !g.spend(len(base), 1, len(name)) {
				return
			}
			identity := base + "#" + name
			if previous := g.anchors[identity]; previous != nil && previous.path != path {
				g.add("schema.anchor.duplicate", g.childPath(path, key), "anchor conflicts with "+previous.path+" is duplicated")
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
		if g.schemaOnly && role == "schema" && (key == "discriminator" || key == "xml") {
			continue
		}
		child := childRole(role, key)
		if child != "" {
			g.collect(m[key], g.childPath(path, key), child, base, document, false)
		}
	}
}

// Require each URI to identify one resource; retrieval and declared identities may match for the same node.
func (g *referenceGraph) addResource(uri string, node *referenceNode) {
	if old := g.resources[uri]; old != nil && old.path != node.path {
		g.add("resource.duplicate", node.path, "resource URI conflicts with "+old.path+" is duplicated: "+uri)
		return
	}
	if !g.resourceNodes[node.path] {
		if len(g.resourceNodes) >= g.maxResources {
			if !g.resourceLimitReported {
				g.add("budget", node.path, "Resource count including embedded $id exceeds the budget")
				g.resourceLimitReported = true
			}
			return
		}
		g.resourceNodes[node.path] = true
	}
	g.resources[uri] = node
}

// Validate the character set permitted for standard anchors.
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

// Enforce a hard reference-count limit and report exhaustion instead of claiming a complete check.
func (g *referenceGraph) addUse(value any, path string, node *referenceNode, role string) {
	if g.budget.exceeded {
		return
	}
	if len(g.uses) >= g.maxReferences {
		if !g.limitReported {
			g.add("budget", path, "reference count exceeds the budget")
			g.limitReported = true
		}
		return
	}
	ref, ok := value.(string)
	if !ok {
		g.add("ref.uri", path, "reference must be a URI-reference string")
		return
	}
	g.uses = append(g.uses, referenceUse{value: ref, path: path, base: node.base, role: role, owner: node.path})
}

// Resolve known discriminator component names directly while preserving URI semantics for other strings.
func (g *referenceGraph) addMapping(value any, path string, node *referenceNode) {
	if g.budget.exceeded {
		return
	}
	ref, ok := value.(string)
	if !ok {
		g.add("ref.uri", path, "mapping must be a component name or URI-reference string")
		return
	}
	// Look up component names in the complete document independently of traversal order.
	doc := g.nodes[node.document]
	if doc != nil {
		root, _ := doc.value.(map[string]any)
		components, _ := root["components"].(map[string]any)
		schemas, _ := components["schemas"].(map[string]any)
		if _, exists := schemas[ref]; exists {
			copy := *node
			copy.base = doc.base
			target := "#/components/schemas/" + escape(ref)
			// Resolve name mappings directly to components, using their $id resource identity when present.
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

// Check initial reference targets without recursively expanding cycles or validating instances.
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

// Resolve target resources, then strictly validate JSON Pointers, anchors, and object kinds.
func (g *referenceGraph) target(use referenceUse) (*referenceNode, string, string) {
	if !g.spend(len(use.base), len(use.value)) {
		return nil, "budget", "reference resolution exceeds the index budget"
	}
	u, err := absoluteReference(use.base, use.value)
	if err != nil {
		return nil, "ref.uri", "reference is not a valid URI-reference: " + use.value
	}
	uri := resourceURI(u)
	if !g.spend(len(uri)) {
		return nil, "budget", "reference resolution exceeds the index budget"
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
		return nil, "external.denied", "required offline reference resource was not provided: " + uri
	}
	var target *referenceNode
	if u.Fragment == "" {
		target = resource
	} else if strings.HasPrefix(u.Fragment, "/") {
		path, code := g.pointerPath(resource, u.Fragment)
		if code != "" {
			return nil, code, "JSON Pointer is invalid or its target does not exist: " + use.value
		}
		target = g.nodes[path]
		if target != nil && target.role == "schema" && target.base != resource.base {
			return nil, "ref.scope", "JSON Pointer crosses the target Schema's $id; use the nearest $id as the reference base"
		}
		if target == nil {
			return nil, "ref.type", "reference targets data or an unknown specification object: " + use.value
		}
	} else {
		if !g.spend(len(resource.base), 1, len(u.Fragment)) {
			return nil, "budget", "anchor resolution exceeds the index budget"
		}
		target = g.anchors[resource.base+"#"+u.Fragment]
	}
	if target == nil {
		return nil, "ref.missing", "reference target does not exist: " + use.value
	}
	if target.role != use.role {
		return nil, "ref.type", "reference target requires " + use.role + ", got " + target.role
	}
	return target, "", ""
}

// Convert URI-decoded JSON Pointers to physical paths using strict escape and array-index syntax.
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
