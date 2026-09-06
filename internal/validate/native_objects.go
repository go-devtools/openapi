package validate

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

// Reject misspelled standard fields while preserving opaque extension data.
func (c *checker) nativeFields(m map[string]any, path, role string, allowed ...string) {
	for _, key := range sortedKeys(m) {
		if c.stopped() {
			return
		}
		if strings.HasPrefix(key, "x-") {
			continue
		}
		known := false
		for _, field := range allowed {
			if key == field {
				known = true
				break
			}
		}
		if !known {
			c.add(role+".field", path+"/"+escape(key), "unknown "+role+" field: "+key)
		}
	}
}

// Check present strings without treating an empty string as absence.
func (c *checker) nativeStrings(m map[string]any, path, role string, fields ...string) {
	for _, key := range fields {
		if c.stopped() {
			return
		}
		if value, present := m[key]; present {
			if _, ok := value.(string); !ok {
				c.add(role+".string", path+"/"+escape(key), key+" must be a string")
			}
		}
	}
}

// Validate example representations without evaluating logical values or fetching external content.
func (c *checker) example(m map[string]any, path string) {
	c.nativeFields(m, path, "example", "summary", "description", "value", "dataValue", "serializedValue", "externalValue")
	c.nativeStrings(m, path, "example", "summary", "description", "serializedValue", "externalValue")
	if has(m, "value") && (has(m, "dataValue") || has(m, "serializedValue") || has(m, "externalValue")) {
		c.add("example.conflict", path, "value is mutually exclusive with OpenAPI 3.2 example representations")
	}
	if has(m, "serializedValue") && has(m, "externalValue") {
		c.add("example.conflict", path, "serializedValue and externalValue are mutually exclusive")
	}
}

// Check discriminator structure; the reference graph resolves schema names and mapping URIs.
func (c *checker) discriminator(m map[string]any, path string) {
	c.nativeFields(m, path, "discriminator", "propertyName", "mapping", "defaultMapping")
	if _, ok := m["propertyName"].(string); !ok {
		c.add("discriminator.propertyName", path+"/propertyName", "propertyName is required and must be a string")
	}
	c.nativeStrings(m, path, "discriminator", "defaultMapping")
	if value, present := m["mapping"]; present {
		mapping, ok := value.(map[string]any)
		if !ok {
			c.add("discriminator.mapping", path+"/mapping", "mapping must be an object with string values")
			return
		}
		for _, key := range sortedKeys(mapping) {
			if c.stopped() {
				return
			}
			if _, ok := mapping[key].(string); !ok {
				c.add("discriminator.mapping", path+"/mapping/"+escape(key), "mapping value must be a schema name or URI-reference string")
			}
		}
	}
}

// Validate XML annotations without selecting or executing an XML codec.
func (c *checker) xml(m map[string]any, path string) {
	c.nativeFields(m, path, "xml", "nodeType", "name", "namespace", "prefix", "attribute", "wrapped")
	c.nativeStrings(m, path, "xml", "name", "namespace", "prefix")
	if value, present := m["nodeType"]; present {
		node, ok := value.(string)
		if !ok || (node != "element" && node != "attribute" && node != "text" && node != "cdata" && node != "none") {
			c.add("xml.nodeType", path+"/nodeType", "nodeType must be element, attribute, text, cdata, or none")
		}
		if has(m, "attribute") || has(m, "wrapped") {
			c.add("xml.conflict", path, "nodeType forbids attribute and wrapped, including explicit false values")
		}
	}
	for _, key := range []string{"attribute", "wrapped"} {
		if value, present := m[key]; present {
			if _, ok := value.(bool); !ok {
				c.add("xml.boolean", path+"/"+key, key+" must be a boolean")
			}
		}
	}
	if namespace, ok := m["namespace"].(string); ok && !nonRelativeIRI(namespace) {
		c.add("xml.namespace", path+"/namespace", "namespace must be a non-relative IRI")
	}
	if has(m, "wrapped") && c.graph != nil {
		node := c.graph.nodes[strings.TrimSuffix(path, "/xml")]
		if node != nil {
			schema, _ := node.value.(map[string]any)
			if !schemaHasType(schema, "array") {
				c.add("xml.wrapped", path+"/wrapped", "wrapped may only be used alongside an array type")
			}
		}
	}
}

// Accept RFC 3987 Unicode and fragments while rejecting relative namespace identifiers.
func nonRelativeIRI(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	query, fragment := strings.IndexByte(value, '?'), strings.IndexByte(value, '#')
	for offset, r := range value {
		if r < 0x80 {
			if r <= 0x20 || r == 0x7f || strings.ContainsRune("<>\"{}|\\^`", r) {
				return false
			}
			continue
		}
		ucs := r >= 0xa0 && r <= 0xd7ff || r >= 0xf900 && r <= 0xfdcf || r >= 0xfdf0 && r <= 0xffef || r >= 0x10000 && r <= 0xefffd && r&0xffff <= 0xfffd && (r < 0xe0000 || r >= 0xe1000)
		private := r >= 0xe000 && r <= 0xf8ff || r >= 0xf0000 && r <= 0xffffd || r >= 0x100000 && r <= 0x10fffd
		inQuery := query >= 0 && offset > query && (fragment < 0 || offset < fragment)
		if !ucs && !(private && inQuery) {
			return false
		}
	}
	u, err := url.Parse(value)
	return err == nil && u.IsAbs()
}
