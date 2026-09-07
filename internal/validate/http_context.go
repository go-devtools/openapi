package validate

import "strconv"

// Identify parameters by name and location without ambiguous concatenated keys.
type parameterIdentity struct{ location, name string }

// Cache offline references and parameter lists within the shared validation budget.
type httpContext struct {
	checker    *checker
	targets    map[string]*referenceNode
	resolved   map[string]*referenceNode
	parameters map[string]map[parameterIdentity]bool
}

// Check inherited parameters and cross-document operation identities after indexing all objects.
func (c *checker) checkHTTPContexts() {
	g := c.graph
	if g == nil || g.schemaOnly || c.stopped() {
		return
	}
	h := httpContext{checker: c, targets: map[string]*referenceNode{}, resolved: map[string]*referenceNode{}, parameters: map[string]map[parameterIdentity]bool{}}
	for _, use := range g.uses {
		if c.stopped() {
			return
		}
		if use.role != "path" && use.role != "parameter" {
			continue
		}
		target, code, _ := g.target(use)
		if code == "" && target != nil && g.spend(64) {
			h.targets[use.owner] = target
		}
	}
	ids := map[string][]*referenceNode{}
	paths := sortedKeys(g.nodes)
	for _, path := range paths {
		if c.stopped() {
			return
		}
		node := g.nodes[path]
		object, _ := node.value.(map[string]any)
		switch node.role {
		case "parameter":
			h.parameter(node)
		case "path":
			h.list(node)
		case "operation":
			h.list(node)
			if id, ok := object["operationId"].(string); ok && g.spend(64) {
				if len(ids[id]) > 0 {
					c.add("operationId.duplicate", path+"/operationId", "operationId conflicts with "+ids[id][0].path)
				}
				ids[id] = append(ids[id], node)
			}
		}
	}
	for _, path := range paths {
		if c.stopped() {
			return
		}
		node := g.nodes[path]
		if node.role == "path" {
			h.path(node)
		}
		if node.role == "link" {
			object, _ := node.value.(map[string]any)
			if id, ok := object["operationId"].(string); ok && !has(object, "$ref") && len(ids[id]) != 1 {
				c.add("link.operationId", path+"/operationId", "operationId must resolve to exactly one operation in the supplied OpenAPI description")
			}
		}
	}
}

// Resolve parameter aliases iteratively and diagnose cycles without a concrete parameter.
func (h *httpContext) parameter(node *referenceNode) *referenceNode {
	if node == nil {
		return nil
	}
	g, c := h.checker.graph, h.checker
	var chain []*referenceNode
	seen := map[string]bool{}
	var result *referenceNode
	for node != nil && !c.stopped() {
		if cached, ok := h.resolved[node.path]; ok {
			result = cached
			break
		}
		if seen[node.path] {
			c.add("parameter.reference.cycle", node.path+"/$ref", "parameter reference cycle has no concrete parameter")
			break
		}
		if !g.spend(128) {
			break
		}
		seen[node.path] = true
		chain = append(chain, node)
		object, ok := node.value.(map[string]any)
		if !ok {
			break
		}
		if !has(object, "$ref") {
			result = node
			break
		}
		node = h.targets[node.path]
	}
	for _, item := range chain {
		h.resolved[item.path] = result
	}
	return result
}

// Reject duplicates within each parameter list before applying operation overrides.
func (h *httpContext) list(owner *referenceNode) map[parameterIdentity]bool {
	if owner == nil {
		return nil
	}
	if cached, ok := h.parameters[owner.path]; ok {
		return cached
	}
	c, g := h.checker, h.checker.graph
	object, _ := owner.value.(map[string]any)
	list, _ := object["parameters"].([]any)
	result := map[parameterIdentity]bool{}
	queries, whole := 0, 0
	for i := range list {
		if c.stopped() {
			return result
		}
		node := h.parameter(g.nodes[owner.path+"/parameters/"+strconv.Itoa(i)])
		if node == nil {
			continue
		}
		value, _ := node.value.(map[string]any)
		name, named := value["name"].(string)
		location, located := value["in"].(string)
		if !named || !located {
			continue
		}
		key := parameterIdentity{location, name}
		if result[key] {
			c.add("parameter.duplicate", owner.path+"/parameters/"+strconv.Itoa(i), "duplicate parameter with the same name and location")
		}
		if !g.spend(64) {
			return result
		}
		result[key] = true
		if location == "query" {
			queries++
		}
		if location == "querystring" {
			whole++
		}
	}
	h.conflict(queries, whole, owner.path+"/parameters")
	if g.spend(64) {
		h.parameters[owner.path] = result
	}
	return result
}

// Prefer the nearest explicit Path Item field and terminate alias cycles iteratively.
func (h *httpContext) path(node *referenceNode) {
	c, g := h.checker, h.checker.graph
	fields := map[string]*referenceNode{}
	template := g.pathTemplates[node.path]
	nonempty, operationCount := false, 0
	seen := map[string]bool{}
	for current := node; current != nil && !seen[current.path] && !c.stopped(); current = h.targets[current.path] {
		if !g.spend(64) {
			return
		}
		seen[current.path] = true
		object, _ := current.value.(map[string]any)
		// Detect content in constant time even when a reused item has many extensions.
		nonempty = nonempty || len(object) > 1 || len(object) == 1 && !has(object, "$ref")
		for _, key := range []string{"parameters", "additionalOperations", "get", "put", "post", "delete", "options", "head", "patch", "trace", "query"} {
			if has(object, key) && fields[key] == nil && g.spend(64) {
				fields[key] = current
			}
		}
	}
	inherited := h.list(fields["parameters"])
	for _, field := range sortedKeys(fields) {
		if c.stopped() {
			return
		}
		owner := fields[field]
		if field == "parameters" {
			continue
		}
		if field == "additionalOperations" {
			object, _ := owner.value.(map[string]any)
			operations, _ := object[field].(map[string]any)
			for _, method := range sortedKeys(operations) {
				if c.stopped() {
					return
				}
				operationCount++
				h.operation(g.nodes[owner.path+"/additionalOperations/"+escape(method)], inherited, node.path, template)
			}
		} else {
			operationCount++
			h.operation(g.nodes[owner.path+"/"+field], inherited, node.path, template)
		}
	}
	// An effectively empty Path Item, including an alias to one, retains the ACL exception.
	if operationCount == 0 && nonempty {
		h.boundParameters(template, inherited, nil, node.path, "")
	}
}

// Override inherited parameters with matching identities while preserving all other entries.
func (h *httpContext) operation(node *referenceNode, inherited map[parameterIdentity]bool, path string, template *pathTemplate) {
	if node == nil {
		return
	}
	own := h.list(node)
	h.boundParameters(template, inherited, own, path, node.path)
	queries, whole := 0, 0
	for _, parameters := range []map[parameterIdentity]bool{inherited, own} {
		for key := range parameters {
			if !h.checker.graph.spend(16) {
				return
			}
			if key.location == "query" {
				queries = 1
			}
		}
	}
	for key := range inherited {
		if key.location == "querystring" && !own[key] {
			whole++
		}
	}
	for key := range own {
		if key.location == "querystring" {
			whole++
		}
	}
	h.conflict(queries, whole, path+"/parameters")
}

// Allow at most one querystring parameter and forbid combining it with query parameters.
func (h *httpContext) conflict(queries, whole int, path string) {
	if whole > 1 || queries > 0 && whole > 0 {
		h.checker.add("parameter.querystring", path, "at most one querystring parameter is allowed, without query parameters, after path inheritance and operation overrides")
	}
}
