package validate

import "strconv"

// 使用名称和位置识别参数，避免覆盖规则依赖串联字符串。
type parameterIdentity struct{ location, name string }

// 缓存离线引用和参数列表，共享检查器的资源预算。
type httpContext struct {
	checker    *checker
	targets    map[string]*referenceNode
	resolved   map[string]*referenceNode
	parameters map[string]map[parameterIdentity]bool
}

// 在全部对象完成索引后检查参数继承与跨文档操作身份。
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

// 迭代解析参数别名，缓存结果并报告没有具体参数的纯引用循环。
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

// 验证单个参数列表；操作级覆盖不允许掩盖列表内部的重复项。
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

// 路径引用采用就近字段优先，迭代访问每个别名以终止循环。
func (h *httpContext) path(node *referenceNode) {
	c, g := h.checker, h.checker.graph
	fields := map[string]*referenceNode{}
	seen := map[string]bool{}
	for current := node; current != nil && !seen[current.path] && !c.stopped(); current = h.targets[current.path] {
		if !g.spend(64) {
			return
		}
		seen[current.path] = true
		object, _ := current.value.(map[string]any)
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
				h.operation(g.nodes[owner.path+"/additionalOperations/"+escape(method)], inherited, node.path)
			}
		} else {
			h.operation(g.nodes[owner.path+"/"+field], inherited, node.path)
		}
	}
}

// 同名同位置的操作参数覆盖继承参数，其余路径参数仍然生效。
func (h *httpContext) operation(node *referenceNode, inherited map[parameterIdentity]bool, path string) {
	if node == nil {
		return
	}
	own := h.list(node)
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

// Querystring 最多一个，并且不能与任何单独的 Query 参数共存。
func (h *httpContext) conflict(queries, whole int, path string) {
	if whole > 1 || queries > 0 && whole > 0 {
		h.checker.add("parameter.querystring", path, "at most one querystring parameter is allowed, without query parameters, after path inheritance and operation overrides")
	}
}
