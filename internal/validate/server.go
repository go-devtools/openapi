package validate

import (
	"strings"
	"unicode/utf8"
)

// Check server metadata and URL-template syntax without contacting the declared host.
func (c *checker) server(m map[string]any, path string) {
	c.nativeFields(m, path, "server", "url", "description", "name", "variables")
	c.nativeStrings(m, path, "server", "description", "name")
	value, ok := m["url"].(string)
	if !ok || value == "" {
		c.add("server.url", path+"/url", "server URL must be a nonempty URL-template string")
		return
	}
	seen := map[string]bool{}
	for i := 0; i < len(value); {
		if c.stopped() {
			return
		}
		if value[i] == '{' {
			end := strings.IndexByte(value[i+1:], '}')
			if end < 0 {
				c.add("server.template", path+"/url", "server variable has no closing brace")
				return
			}
			name := value[i+1 : i+1+end]
			if name == "" || strings.ContainsRune(name, '{') || seen[name] {
				c.add("server.template", path+"/url", "server variables must be nonempty, balanced, and appear only once")
				return
			}
			if c.graph != nil && !c.graph.spend(64) {
				return
			}
			seen[name] = true
			i += end + 2
			continue
		}
		if value[i] == '}' {
			c.add("server.template", path+"/url", "server variable has an unmatched closing brace")
			return
		}
		if value[i] == '?' || value[i] == '#' {
			c.add("server.url", path+"/url", "server URLs must not contain a query or fragment")
			return
		}
		if value[i] == '%' {
			if i+2 >= len(value) || !strings.ContainsRune("0123456789abcdefABCDEF", rune(value[i+1])) || !strings.ContainsRune("0123456789abcdefABCDEF", rune(value[i+2])) {
				c.add("server.url", path+"/url", "server URL contains an invalid percent-encoded triple")
				return
			}
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		if !serverLiteral(r) {
			c.add("server.url", path+"/url", "server URL contains a character forbidden by the URL-template grammar")
			return
		}
		i += size
	}
}

// Validate defaults and enumerations independently of JSON Schema's annotation-only default keyword.
func (c *checker) serverVariable(m map[string]any, path string) {
	c.nativeFields(m, path, "serverVariable", "default", "description", "enum")
	c.nativeStrings(m, path, "serverVariable", "description")
	defaultValue, validDefault := m["default"].(string)
	if !validDefault {
		c.add("serverVariable.default", path+"/default", "server variable default is a required string")
	}
	if value, present := m["enum"]; present {
		values, ok := value.([]any)
		if !ok || len(values) == 0 {
			c.add("serverVariable.enum", path+"/enum", "server variable enum must be a nonempty array of strings")
			return
		}
		found := false
		for _, value := range values {
			if c.stopped() {
				return
			}
			item, ok := value.(string)
			if !ok {
				c.add("serverVariable.enum", path+"/enum", "server variable enum values must be strings")
				continue
			}
			found = found || item == defaultValue
		}
		if validDefault && !found {
			c.add("serverVariable.default", path+"/default", "server variable default must occur in its enum")
		}
	}
}

// Recognize RFC 6570 literal Unicode ranges, including private-use characters, without accepting noncharacters.
func serverLiteral(r rune) bool {
	if r < 0x80 {
		return r > 0x20 && r != 0x7f && !strings.ContainsRune("\"%<>\\^`{|}", r)
	}
	ucs := r >= 0xa0 && r <= 0xd7ff || r >= 0xf900 && r <= 0xfdcf || r >= 0xfdf0 && r <= 0xffef || r >= 0x10000 && r <= 0xefffd && r&0xffff <= 0xfffd && (r < 0xe0000 || r >= 0xe1000)
	private := r >= 0xe000 && r <= 0xf8ff || r >= 0xf0000 && r <= 0xffffd || r >= 0x100000 && r <= 0x10fffd
	return ucs || private
}
