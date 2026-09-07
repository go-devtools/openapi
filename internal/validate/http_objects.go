package validate

import "strings"

// Validate present booleans without collapsing explicit false into absence.
func (c *checker) nativeBooleans(m map[string]any, path, role string, fields ...string) {
	for _, key := range fields {
		if c.stopped() {
			return
		}
		if value, present := m[key]; present {
			if _, ok := value.(bool); !ok {
				c.add(role+".boolean", path+"/"+key, key+" must be a boolean")
			}
		}
	}
}

// Apply the standard location-specific parameter fields, independently of application codecs.
func (c *checker) parameterFields(m map[string]any, path, role string) {
	allowed := []string{"description", "required", "deprecated", "schema", "content", "example", "examples"}
	location := str(m, "in")
	if role == "parameter" {
		allowed = append(allowed, "name", "in")
		if location == "query" {
			allowed = append(allowed, "allowEmptyValue")
		}
	}
	if has(m, "schema") {
		allowed = append(allowed, "style", "explode")
		if role == "parameter" && (location == "path" || location == "query" || (location == "cookie" && (!has(m, "style") || str(m, "style") == "form"))) {
			allowed = append(allowed, "allowReserved")
		}
	}
	c.nativeFields(m, path, role, allowed...)
	c.nativeStrings(m, path, role, "description", "style")
	c.nativeBooleans(m, path, role, "required", "deprecated", "allowEmptyValue", "explode", "allowReserved")
	if has(m, "schema") && has(m, "style") {
		style := str(m, "style")
		valid := false
		if role == "header" {
			location = "header"
		}
		switch location {
		case "path":
			valid = style == "matrix" || style == "label" || style == "simple"
		case "query":
			valid = style == "form" || style == "spaceDelimited" || style == "pipeDelimited" || style == "deepObject"
		case "header":
			valid = style == "simple"
		case "cookie":
			valid = style == "form" || style == "cookie"
		}
		if !valid {
			c.add("parameter.style", path+"/style", "style is not defined for this parameter location")
		}
	}
}

// Keep native operations optional while validating every explicitly supplied descriptive field.
func (c *checker) operationFields(m map[string]any, path string) {
	c.nativeFields(m, path, "operation", "tags", "summary", "description", "externalDocs", "operationId", "parameters", "requestBody", "responses", "callbacks", "deprecated", "security", "servers")
	c.nativeStrings(m, path, "operation", "summary", "description", "operationId")
	c.nativeBooleans(m, path, "operation", "deprecated")
	if value, present := m["tags"]; present {
		values, ok := value.([]any)
		if !ok {
			c.add("operation.tags", path+"/tags", "operation tags must be an array of strings")
			return
		}
		for _, value := range values {
			if c.stopped() {
				return
			}
			if _, ok := value.(string); !ok {
				c.add("operation.tags", path+"/tags", "operation tags must contain only strings")
				return
			}
		}
	}
}

// Keep Link parameters and request bodies opaque because their values may be literal business data.
func (c *checker) linkFields(m map[string]any, path string) {
	c.nativeFields(m, path, "link", "operationRef", "operationId", "parameters", "requestBody", "description", "server")
	c.nativeStrings(m, path, "link", "operationRef", "operationId", "description")
	if value, present := m["parameters"]; present {
		if _, ok := value.(map[string]any); !ok {
			c.add("link.parameters", path+"/parameters", "link parameters must be a map of literal values or runtime expressions")
		}
	}
}

// Preserve case because HTTP method tokens are case-sensitive.
func httpToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}
