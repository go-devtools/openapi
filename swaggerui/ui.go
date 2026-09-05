// 提供固定版本、离线可用且不绑定 HTTP 框架的 Swagger UI 资源。
package swaggerui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/url"
	"path"
	"sort"
	"strings"
)

// 标识经过固定校验的上游发行版本。
const Version = "5.32.15"

// 静态资源只会在显式导入 swaggerui 子包时链接。
//
//go:embed assets/*
var assets embed.FS

// 对固定上游版本补充展示名称，不修改其发行资源。
//
//go:embed display-names.js
var displayNames string

// 本项目的表现层样式与原始上游资源分别保存。
//
//go:embed presentation.css
var presentationStyles string

// 从显式配置启动页面，并仅恢复已注册的分类名称。
//
//go:embed startup.js
var startupScript string

// 独立配置页面标题、相对文档地址及明确允许的提交方法。
type Config struct {
	Title         string
	SpecURL       string
	SubmitMethods []string
	// 启用按标签筛选，并选择标签或接口的默认展开与排序方式。
	Filter           bool
	DocExpansion     string
	TagsSorter       string
	OperationsSorter string
	// 多份文档时启用右上角分类选择器，默认选中第一份。
	Definitions       []Definition
	PrimaryDefinition string
}

// 标记可切换的整份本地文档，名称仅用于选择器展示。
type Definition struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// 保存只读资源，传输层不需要采用 net/http。
type UI struct{ resources map[string]Resource }

// 保存不可变内容和响应元数据，所有集合访问都返回副本。
type Resource struct {
	body         string
	contentType  string
	etag         string
	cacheControl string
}

// 返回资源内容的防御性副本。
func (r Resource) Bytes() []byte { return []byte(r.body) }

// 返回适用于任意传输适配器的安全与缓存元数据。
func (r Resource) Headers() map[string]string {
	return map[string]string{"Content-Type": r.contentType, "ETag": r.etag, "Cache-Control": r.cacheControl, "X-Content-Type-Options": "nosniff", "Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'", "Referrer-Policy": "no-referrer"}
}

// 返回媒体类型，框架可以直接复用而不重新猜测。
func (r Resource) ContentType() string { return r.contentType }

// 构造经过 HTML/JSON 编码的页面，不插入未编码业务文字。
func New(cfg Config) (*UI, error) {
	if cfg.Title == "" {
		cfg.Title = "API Documentation"
	}
	if cfg.SpecURL == "" {
		cfg.SpecURL = "./openapi.json"
	}
	if err := localSpecURL(cfg.SpecURL); err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, definition := range cfg.Definitions {
		if definition.Name == "" || names[definition.Name] {
			return nil, fmt.Errorf("openapi.ui.definition: 文档分类名称为空或重复")
		}
		names[definition.Name] = true
		if err := localSpecURL(definition.URL); err != nil {
			return nil, err
		}
	}
	if cfg.PrimaryDefinition != "" && !names[cfg.PrimaryDefinition] {
		return nil, fmt.Errorf("openapi.ui.definition: 默认分类不存在")
	}
	methods := append([]string{}, cfg.SubmitMethods...)
	for _, method := range methods {
		switch method {
		case "get", "put", "post", "delete", "options", "head", "patch", "trace", "query":
		default:
			return nil, fmt.Errorf("openapi.ui.method: 未验收的提交方法 %s", method)
		}
	}
	if cfg.DocExpansion == "" {
		cfg.DocExpansion = "list"
	}
	if cfg.DocExpansion != "none" && cfg.DocExpansion != "list" && cfg.DocExpansion != "full" {
		return nil, fmt.Errorf("openapi.ui.expansion: 未知分组展开方式")
	}
	if cfg.TagsSorter != "" && cfg.TagsSorter != "alpha" {
		return nil, fmt.Errorf("openapi.ui.sort: 未知标签排序")
	}
	if cfg.OperationsSorter != "" && cfg.OperationsSorter != "alpha" && cfg.OperationsSorter != "method" {
		return nil, fmt.Errorf("openapi.ui.sort: 未知接口排序")
	}
	ui := &UI{resources: map[string]Resource{}}
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		raw, err := assets.ReadFile("assets/" + entry.Name())
		if err != nil {
			return nil, err
		}
		contentType := mime.TypeByExtension(path.Ext(entry.Name()))
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		ui.resources[entry.Name()] = resource(raw, contentType, "public, max-age=86400")
	}
	configuration := map[string]any{"url": cfg.SpecURL, "dom_id": "#swagger-ui", "validatorUrl": nil, "persistAuthorization": false, "queryConfigEnabled": false, "supportedSubmitMethods": methods, "tryItOutEnabled": len(methods) > 0, "deepLinking": true, "displayRequestDuration": true, "useUnsafeMarkdown": false, "filter": cfg.Filter, "docExpansion": cfg.DocExpansion, "defaultModelExpandDepth": 2}
	if len(cfg.Definitions) > 0 {
		delete(configuration, "url")
		configuration["urls"] = cfg.Definitions
		configuration["layout"] = "StandaloneLayout"
		primary := cfg.PrimaryDefinition
		if primary == "" {
			primary = cfg.Definitions[0].Name
		}
		configuration["urls.primaryName"] = primary
	}
	if cfg.TagsSorter != "" {
		configuration["tagsSorter"] = cfg.TagsSorter
	}
	if cfg.OperationsSorter != "" {
		configuration["operationsSorter"] = cfg.OperationsSorter
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return nil, err
	}
	script := "// 从固定本地配置启动文档。\nwindow.addEventListener('load', function () { window.OpenAPIStart(" + string(encoded) + "); });\n"
	page := "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>" + html.EscapeString(cfg.Title) + "</title><link rel=\"stylesheet\" href=\"./swagger-ui.css\"><link rel=\"stylesheet\" href=\"./presentation.css\"><link rel=\"icon\" href=\"./favicon-32x32.png\"></head><body><div id=\"swagger-ui\"></div><script src=\"./swagger-ui-bundle.js\"></script><script src=\"./swagger-ui-standalone-preset.js\"></script><script src=\"./display-names.js\"></script><script src=\"./startup.js\"></script><script src=\"./config.js\"></script></body></html>"
	ui.resources["presentation.css"] = resource([]byte(presentationStyles), "text/css; charset=utf-8", "no-cache")
	ui.resources["display-names.js"] = resource([]byte(displayNames), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["startup.js"] = resource([]byte(startupScript), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["config.js"] = resource([]byte(script), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["index.html"] = resource([]byte(page), "text/html; charset=utf-8", "no-cache")
	return ui, nil
}

// 计算固定 ETag，使资源请求只读取缓存字节。
func resource(raw []byte, contentType, cacheControl string) Resource {
	sum := sha256.Sum256(raw)
	return Resource{body: string(raw), contentType: contentType, etag: "\"" + hex.EncodeToString(sum[:]) + "\"", cacheControl: cacheControl}
}

// 严格读取单个命名资源，拒绝编码绕过与目录穿越。
func (u *UI) Resource(name string) (Resource, error) {
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || strings.Contains(name, "%") || path.Clean(name) != name {
		return Resource{}, fmt.Errorf("openapi.ui.path: 非法资源路径")
	}
	r, ok := u.resources[name]
	if !ok {
		return Resource{}, fs.ErrNotExist
	}
	return r, nil
}

// 返回稳定排序的资源名称副本，方便非 HTTP 消费者预载。
func (u *UI) Names() []string {
	out := make([]string, 0, len(u.resources))
	for name := range u.resources {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// 所有规范加载入口都限制为同源路径，编码后的父级跳转也不能绕过校验。
func localSpecURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || value == "" || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Contains(parsed.Path, "..") || strings.HasPrefix(value, "//") || strings.ContainsAny(parsed.Path, "\\\r\n") {
		return fmt.Errorf("openapi.ui.specURL: 只允许本地规范地址")
	}
	return nil
}
