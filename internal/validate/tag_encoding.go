package validate

import "strconv"

// Check tag fields without restricting names or user-defined classification strings.
func (c *checker) tag(m map[string]any, path string) {
	c.nativeFields(m, path, "tag", "name", "summary", "description", "parent", "kind", "externalDocs")
	if _, ok := m["name"].(string); !ok {
		c.add("tag.name", path+"/name", "tag name is a required string")
	}
	c.nativeStrings(m, path, "tag", "summary", "description", "parent", "kind")
}

// Validate documentation metadata as a link, without reading or fetching its target.
func (c *checker) externalDocs(m map[string]any, path string) {
	c.nativeFields(m, path, "externalDocs", "url", "description")
	c.nativeStrings(m, path, "externalDocs", "description")
	value, ok := m["url"].(string)
	if !ok {
		c.add("externalDocs.url", path+"/url", "documentation URL is a required URI-reference string")
		return
	}
	if _, err := parseURIReference(value); err != nil {
		c.add("externalDocs.url", path+"/url", "documentation URL must be a valid URI-reference")
	}
}

// Validate native media and encoding fields; traversal checks each nested object separately.
func (c *checker) mediaEncoding(m map[string]any, path, role string) {
	if role == "media" {
		c.nativeFields(m, path, role, "description", "schema", "itemSchema", "example", "examples", "encoding", "prefixEncoding", "itemEncoding")
		c.nativeStrings(m, path, role, "description")
		return
	}
	c.nativeFields(m, path, role, "contentType", "headers", "style", "explode", "allowReserved", "encoding", "prefixEncoding", "itemEncoding")
	c.nativeStrings(m, path, role, "contentType")
	if style, present := m["style"]; present {
		switch style {
		case "form", "spaceDelimited", "pipeDelimited", "deepObject":
		default:
			c.add("encoding.style", path+"/style", "encoding style must be form, spaceDelimited, pipeDelimited, or deepObject")
		}
	}
	for _, field := range []string{"explode", "allowReserved"} {
		if value, present := m[field]; present {
			if _, ok := value.(bool); !ok {
				c.add("encoding.boolean", path+"/"+field, field+" must be a boolean")
			}
		}
	}
	if value, present := m["headers"]; present {
		if _, ok := value.(map[string]any); !ok {
			c.add("encoding.headers", path+"/headers", "encoding headers must be a map of Header or Reference Objects")
		}
	}
}

// Find structural array evidence through ordinary references and positive compositions, without solving instances.
func (c *checker) positionalArray(path string) bool {
	g := c.graph
	if g == nil {
		return false
	}
	queue := []*referenceNode{}
	seen := map[string]bool{}
	// Charge retained state before enqueuing each physical schema once, including reference cycles.
	enqueue := func(node *referenceNode) {
		if node != nil && node.role == "schema" && !seen[node.path] && g.spend(64) {
			seen[node.path] = true
			queue = append(queue, node)
		}
	}
	enqueue(g.nodes[path])
	for i := 0; i < len(queue) && !c.stopped(); i++ {
		node := queue[i]
		object, ok := node.value.(map[string]any)
		if !ok {
			continue
		}
		if schemaHasType(object, "array") {
			return true
		}
		if has(object, "type") {
			continue
		}
		if has(object, "items") || has(object, "prefixItems") {
			return true
		}
		if value, ok := object["$ref"].(string); ok {
			target, code, _ := g.target(referenceUse{value: value, path: node.path + "/$ref", base: node.base, role: "schema", owner: node.path})
			if code == "" {
				enqueue(target)
			}
		}
		for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
			entries, _ := object[keyword].([]any)
			for j := range entries {
				if c.stopped() {
					return false
				}
				enqueue(g.nodes[g.childPath(g.childPath(node.path, keyword), strconv.Itoa(j))])
			}
		}
	}
	return false
}
