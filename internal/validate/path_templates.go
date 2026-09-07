package validate

import "strings"

// Keep literal hierarchy separate from the exact, case-sensitive parameter names.
type pathTemplate struct {
	shape string
	names map[string]bool
}

// Index only actual Paths entries; component names, callbacks and webhooks are not endpoint templates.
func (c *checker) checkPaths(paths map[string]any, parent string) {
	shapes := map[string]string{}
	for _, path := range sortedKeys(paths) {
		if c.stopped() {
			return
		}
		if dictionaryExtension("paths", path) {
			continue
		}
		// Charge parsing, canonical text and per-expression bookkeeping before allocation.
		if !c.graph.spend(len(path), len(path), 64, 64*strings.Count(path, "{")) {
			return
		}
		location := c.graph.childPath(parent, path)
		if c.stopped() {
			return
		}
		template, problem := parsePathTemplate(path)
		if problem != "" {
			c.add("path", location, problem)
			continue
		}
		if previous, exists := shapes[template.shape]; exists {
			c.add("paths.identical", location, "path template has the same hierarchy as "+previous)
		} else {
			shapes[template.shape] = location
		}
		c.graph.pathTemplates[location] = template
	}
}

// Apply the OpenAPI 3.2 path ABNF without percent-decoding literals or restricting expression names.
func parsePathTemplate(path string) (*pathTemplate, string) {
	if !strings.HasPrefix(path, "/") {
		return nil, "path must begin with a slash"
	}
	template := &pathTemplate{names: map[string]bool{}}
	var shape strings.Builder
	shape.Grow(len(path))
	shape.WriteByte('/')
	segment := false
	for i := 1; i < len(path); {
		ch := path[i]
		switch ch {
		case '/':
			if !segment {
				return nil, "path segments must not be empty"
			}
			shape.WriteByte(ch)
			segment = false
			i++
		case '{':
			start := i + 1
			i = start
			for i < len(path) && path[i] != '}' {
				if path[i] == '{' {
					return nil, "path templates must not be nested"
				}
				i++
			}
			if i == len(path) {
				return nil, "path template is unterminated"
			}
			if i == start {
				return nil, "path template parameter name must not be empty"
			}
			name := path[start:i]
			if template.names[name] {
				return nil, "path template expressions must not repeat"
			}
			template.names[name] = true
			shape.WriteString("{}")
			segment = true
			i++
		case '%':
			if i+2 >= len(path) || !hexDigit(path[i+1]) || !hexDigit(path[i+2]) {
				return nil, "path literal percent escapes require two hexadecimal digits"
			}
			shape.WriteString(path[i : i+3])
			segment = true
			i += 3
		default:
			if !pathLiteralByte(ch) {
				return nil, "path literals must use RFC 3986 pchar characters or percent encoding"
			}
			shape.WriteByte(ch)
			segment = true
			i++
		}
	}
	template.shape = shape.String()
	return template, ""
}

// Accept ASCII unreserved characters, sub-delimiters, colon and at-sign outside expressions.
func pathLiteralByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("-._~!$&'()*+,;=:@", rune(ch))
}

// Require all and only the path names in the bound endpoint after parameter inheritance.
func (h *httpContext) boundParameters(template *pathTemplate, inherited, own map[parameterIdentity]bool, location, operation string) {
	if template == nil {
		return
	}
	c, g := h.checker, h.checker.graph
	if !g.spend(len(operation), 32) {
		return
	}
	scope := ""
	if operation != "" {
		scope = " (operation " + operation + ")"
	}
	names := map[string]bool{}
	for _, parameters := range []map[parameterIdentity]bool{inherited, own} {
		for key := range parameters {
			if !g.spend(64, len(key.name)) {
				return
			}
			if key.location == "path" {
				names[key.name] = true
			}
		}
	}
	for _, name := range sortedKeys(names) {
		if c.stopped() {
			return
		}
		if !template.names[name] {
			c.add("parameter.path.unused", location, "path parameter does not occur in this endpoint template: "+name+scope)
		}
	}
	for _, name := range sortedKeys(template.names) {
		if !g.spend(32, len(name)) {
			return
		}
		if !names[name] {
			c.add("parameter.path.missing", location, "endpoint template requires a path parameter in the Path Item or each operation: "+name+scope)
		}
	}
}
