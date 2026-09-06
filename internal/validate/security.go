package validate

import "strings"

// Validate fields applicable to each security scheme without interpreting extension data.
// 验证各安全方案适用的字段，不解释扩展数据。
func (c *checker) security(m map[string]any, path string) {
	allowed := []string{"type", "description", "deprecated"}
	switch str(m, "type") {
	case "apiKey":
		allowed = append(allowed, "name", "in")
		if str(m, "name") == "" || (str(m, "in") != "header" && str(m, "in") != "query" && str(m, "in") != "cookie") {
			c.add("security.apiKey", path, "apiKey requires a name and header, query, or cookie location")
		}
	case "http":
		allowed = append(allowed, "scheme")
		if str(m, "scheme") == "" {
			c.add("security.http", path+"/scheme", "HTTP authentication requires a scheme")
		}
		if strings.EqualFold(str(m, "scheme"), "bearer") {
			allowed = append(allowed, "bearerFormat")
		}
	case "oauth2":
		allowed = append(allowed, "flows", "oauth2MetadataUrl")
		flows, ok := m["flows"].(map[string]any)
		if !ok {
			c.add("security.oauth2", path+"/flows", "OAuth2 requires a flows object, including when oauth2MetadataUrl is present")
		} else {
			c.oauthFlows(flows, path+"/flows")
		}
		if has(m, "oauth2MetadataUrl") {
			c.securityURL(m, "oauth2MetadataUrl", path, "")
		}
	case "openIdConnect":
		allowed = append(allowed, "openIdConnectUrl")
		c.securityURL(m, "openIdConnectUrl", path, "security.openId")
	case "mutualTLS":
	default:
		c.add("security.type", path+"/type", "unknown security scheme type")
	}
	c.securityFields(m, path, allowed...)
	for _, key := range []string{"description", "bearerFormat"} {
		if value, exists := m[key]; exists {
			if _, ok := value.(string); !ok {
				c.add("security.string", path+"/"+key, key+" must be a string")
			}
		}
	}
	if value, exists := m["deprecated"]; exists {
		if _, ok := value.(bool); !ok {
			c.add("security.deprecated", path+"/deprecated", "deprecated must be a boolean")
		}
	}
}

// Check all five OAuth flows while permitting empty objects and namespaced extensions.
// 检查五种 OAuth 流程，允许空流程集合和命名空间扩展。
func (c *checker) oauthFlows(flows map[string]any, path string) {
	for _, kind := range sortedKeys(flows) {
		if c.stopped() {
			return
		}
		if strings.HasPrefix(kind, "x-") {
			continue
		}
		location := path + "/" + escape(kind)
		var required []string
		switch kind {
		case "implicit":
			required = []string{"authorizationUrl"}
		case "password", "clientCredentials":
			required = []string{"tokenUrl"}
		case "authorizationCode":
			required = []string{"authorizationUrl", "tokenUrl"}
		case "deviceAuthorization":
			required = []string{"deviceAuthorizationUrl", "tokenUrl"}
		default:
			c.add("security.flow", location, "unknown OAuth flow; custom fields must use an x- extension")
			continue
		}
		flow, ok := flows[kind].(map[string]any)
		if !ok {
			c.add("security.flow", location, "OAuth flow must be an object")
			continue
		}
		c.securityFields(flow, location, append([]string{"scopes", "refreshUrl"}, required...)...)
		missingCode := "security.flow.required"
		if kind == "deviceAuthorization" {
			missingCode = "security.device"
		}
		for _, key := range required {
			c.securityURL(flow, key, location, missingCode)
		}
		if has(flow, "refreshUrl") {
			c.securityURL(flow, "refreshUrl", location, "")
		}
		scopes, ok := flow["scopes"].(map[string]any)
		if !ok {
			c.add("security.scopes", location+"/scopes", "OAuth flow requires a scopes object; use an empty object when no scopes are available")
			continue
		}
		for _, name := range sortedKeys(scopes) {
			if c.stopped() {
				return
			}
			if _, ok := scopes[name].(string); !ok {
				c.add("security.scopes", location+"/scopes/"+escape(name), "OAuth scope description must be a string")
			}
		}
	}
}

// Reject misspelled or inapplicable standard fields while keeping x- values opaque.
// 拒绝拼写错误或不适用的标准字段，保留不透明的 x- 扩展值。
func (c *checker) securityFields(m map[string]any, path string, allowed ...string) {
	for _, key := range sortedKeys(m) {
		if c.stopped() {
			return
		}
		if strings.HasPrefix(key, "x-") {
			continue
		}
		found := false
		for _, candidate := range allowed {
			found = found || key == candidate
		}
		if !found {
			c.add("security.field", path+"/"+escape(key), "field is not defined for this security scheme or OAuth flow")
		}
	}
}

// Check URL syntax and explicit TLS violations without fetching endpoints or guessing runtime servers.
// 检查 URL 语法和显式 TLS 违规，不抓取端点或猜测运行时服务器。
func (c *checker) securityURL(m map[string]any, key, path, missingCode string) {
	value, exists := m[key]
	if !exists {
		if missingCode != "" {
			c.add(missingCode, path+"/"+key, key+" is required for this security scheme or OAuth flow")
		}
		return
	}
	text, ok := value.(string)
	if !ok {
		c.add("security.url", path+"/"+key, key+" must be a URL string")
		return
	}
	u, err := parseURIReference(text)
	if err != nil {
		c.add("security.url", path+"/"+key, key+" must be a valid URL reference")
		return
	}
	if u.IsAbs() && (!strings.EqualFold(u.Scheme, "https") || u.Hostname() == "") {
		c.add("security.url", path+"/"+key, key+" must use HTTPS for an absolute authorization URL")
	}
}
