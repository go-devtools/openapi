package validate

import (
	"net/netip"
	"strings"
)

// Validate root metadata without applying framework or runtime routing rules.
func (c *checker) rootMetadata(m map[string]any, path string) {
	c.nativeFields(m, path, "root", "openapi", "$self", "info", "jsonSchemaDialect", "servers", "paths", "webhooks", "components", "security", "tags", "externalDocs")
	c.nativeStrings(m, path, "root", "openapi", "$self", "jsonSchemaDialect")
	c.metadataURI(m, path, "root", "jsonSchemaDialect")
	if !has(m, "paths") && !has(m, "components") && !has(m, "webhooks") {
		c.add("root.required", path, "at least one of paths, components, or webhooks must be present")
	}
}

// Preserve required empty strings while distinguishing absence from invalid field types.
func (c *checker) requiredMetadataStrings(m map[string]any, path, role string, keys ...string) {
	c.nativeStrings(m, path, role, keys...)
	for _, key := range keys {
		if c.stopped() {
			return
		}
		if !has(m, key) {
			c.add(role+".required", path+"/"+key, key+" is required")
		}
	}
}

// Validate descriptive fields and nested objects without interpreting their Markdown as instructions.
func (c *checker) info(m map[string]any, path string) {
	c.nativeFields(m, path, "info", "title", "summary", "description", "termsOfService", "contact", "license", "version")
	c.requiredMetadataStrings(m, path, "info", "title", "version")
	c.nativeStrings(m, path, "info", "summary", "description", "termsOfService")
	c.metadataURI(m, path, "info", "termsOfService")
}

// Check contact syntax without DNS lookups, mailbox delivery, or URI retrieval.
func (c *checker) contact(m map[string]any, path string) {
	c.nativeFields(m, path, "contact", "name", "url", "email")
	c.nativeStrings(m, path, "contact", "name", "url", "email")
	c.metadataURI(m, path, "contact", "url")
	if email, ok := m["email"].(string); ok && !metadataEmail(email) {
		c.add("contact.email", path+"/email", "email must be a single mailbox address without a display name")
	}
}

// Keep license alternatives exclusive by presence, including explicitly empty values.
func (c *checker) license(m map[string]any, path string) {
	c.nativeFields(m, path, "license", "name", "identifier", "url")
	c.requiredMetadataStrings(m, path, "license", "name")
	c.nativeStrings(m, path, "license", "identifier", "url")
	c.metadataURI(m, path, "license", "url")
	if has(m, "identifier") && has(m, "url") {
		c.add("license.conflict", path, "license identifier and url are mutually exclusive")
	}
}

// Validate all component dictionaries while leaving schema property and definition names unrestricted.
func (c *checker) components(m map[string]any, path string) {
	fields := []string{"schemas", "responses", "parameters", "examples", "requestBodies", "headers", "securitySchemes", "links", "callbacks", "pathItems", "mediaTypes"}
	c.nativeFields(m, path, "components", fields...)
	for _, kind := range fields {
		values, _ := m[kind].(map[string]any)
		for _, name := range sortedKeys(values) {
			if c.stopped() {
				return
			}
			if !componentName(name) {
				c.add("components.name", path+"/"+kind+"/"+escape(name), "component names must match ^[a-zA-Z0-9.\\-_]+$")
			}
		}
	}
}

// Apply the same ASCII identifier alphabet to every component family.
func componentName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// Accept relative metadata references and reject malformed percent escapes in any URI component.
func (c *checker) metadataURI(m map[string]any, path, role, key string) {
	value, ok := m[key].(string)
	if !ok {
		return
	}
	_, err := parseURIReference(value)
	valid := err == nil
	for i := 0; i < len(value) && valid; i++ {
		if value[i] == '%' {
			if i+2 >= len(value) || !hexDigit(value[i+1]) || !hexDigit(value[i+2]) {
				valid = false
			} else {
				i += 2
			}
		}
	}
	if !valid {
		c.add(role+".uri", path+"/"+key, key+" must be a valid URI-reference")
	}
}

// Recognize an ASCII hexadecimal digit without accepting Unicode lookalikes.
func hexDigit(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

// Validate an ASCII mailbox with quoted local parts and IPv4 or IPv6 address literals.
func metadataEmail(value string) bool {
	at := strings.LastIndexByte(value, '@')
	if at < 1 || len(value) > 254 {
		return false
	}
	local, domain := value[:at], value[at+1:]
	if len(local) > 64 || domain == "" {
		return false
	}
	if local[0] == '"' {
		if len(local) < 2 || local[len(local)-1] != '"' {
			return false
		}
		for i := 1; i < len(local)-1; i++ {
			b := local[i]
			if b == '\\' {
				i++
				if i >= len(local)-1 {
					return false
				}
				b = local[i]
			} else if b == '"' {
				return false
			}
			if b < 32 || b > 126 {
				return false
			}
		}
	} else {
		if local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
			return false
		}
		for _, r := range local {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(".!#$%&'*+-/=?^_`{|}~", r)) {
				return false
			}
		}
	}
	if strings.HasPrefix(domain, "[") && strings.HasSuffix(domain, "]") {
		literal := domain[1 : len(domain)-1]
		if v6, ok := strings.CutPrefix(literal, "IPv6:"); ok {
			ip, err := netip.ParseAddr(v6)
			return err == nil && ip.Is6() && ip.Zone() == ""
		}
		ip, err := netip.ParseAddr(literal)
		return err == nil && ip.Is4()
	}
	domain = strings.TrimSuffix(domain, ".")
	if len(domain) > 253 {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
