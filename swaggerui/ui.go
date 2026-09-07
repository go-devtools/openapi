// Provide pinned offline Swagger UI resources independently of HTTP frameworks.
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

// Identify the verified upstream release.
const Version = "5.32.15"

// Link static assets only when the swaggerui subpackage is imported.
//
//go:embed assets/*
var assets embed.FS

// Extend display names without modifying pinned upstream assets.
//
//go:embed display-names.js
var displayNames string

// Render native Example fields without modifying the embedded specification.
//
//go:embed native-examples.js
var nativeExamples string

// Report measured native display limitations without rewriting the document.
//
//go:embed native-compatibility.js
var nativeCompatibility string

// Keep project presentation styles separate from upstream resources.
//
//go:embed presentation.css
var presentationStyles string

// Start the page from explicit configuration and restore only registered document names.
//
//go:embed startup.js
var startupScript string

// Configure the page title, local specification URL, and explicitly allowed methods.
type Config struct {
	Title         string
	SpecURL       string
	SubmitMethods []string
	// Configure tag filtering and default expansion and sorting for tags and operations.
	Filter           bool
	DocExpansion     string
	TagsSorter       string
	OperationsSorter string
	// Enable the top-right selector for multiple documents, selecting the first by default.
	Definitions       []Definition
	PrimaryDefinition string
}

// Identify a switchable local document whose name is used only for display.
type Definition struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Store read-only resources without requiring net/http.
type UI struct{ resources map[string]Resource }

// Store immutable resource content and metadata with defensive-copy access.
type Resource struct {
	body         string
	contentType  string
	etag         string
	cacheControl string
}

// Return a defensive copy of resource bytes.
func (r Resource) Bytes() []byte { return []byte(r.body) }

// Return security and cache metadata for any transport adapter.
func (r Resource) Headers() map[string]string {
	return map[string]string{"Content-Type": r.contentType, "ETag": r.etag, "Cache-Control": r.cacheControl, "X-Content-Type-Options": "nosniff", "Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'", "Referrer-Policy": "no-referrer"}
}

// Return the resource media type without requiring adapters to guess it.
func (r Resource) ContentType() string { return r.contentType }

// Encode HTML and JSON safely when constructing the page.
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
			return nil, fmt.Errorf("openapi.ui.definition: definition name is empty or duplicated")
		}
		names[definition.Name] = true
		if err := localSpecURL(definition.URL); err != nil {
			return nil, err
		}
	}
	if cfg.PrimaryDefinition != "" && !names[cfg.PrimaryDefinition] {
		return nil, fmt.Errorf("openapi.ui.definition: default definition does not exist")
	}
	methods := append([]string{}, cfg.SubmitMethods...)
	for _, method := range methods {
		switch method {
		case "get", "put", "post", "delete", "options", "head", "patch", "trace", "query":
		default:
			return nil, fmt.Errorf("openapi.ui.method: unsupported submit method %s", method)
		}
	}
	if cfg.DocExpansion == "" {
		cfg.DocExpansion = "list"
	}
	if cfg.DocExpansion != "none" && cfg.DocExpansion != "list" && cfg.DocExpansion != "full" {
		return nil, fmt.Errorf("openapi.ui.expansion: unknown group expansion mode")
	}
	if cfg.TagsSorter != "" && cfg.TagsSorter != "alpha" {
		return nil, fmt.Errorf("openapi.ui.sort: unknown tag sort order")
	}
	if cfg.OperationsSorter != "" && cfg.OperationsSorter != "alpha" && cfg.OperationsSorter != "method" {
		return nil, fmt.Errorf("openapi.ui.sort: unknown operation sort order")
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
	script := "// Start documentation from fixed local configuration.\nwindow.addEventListener('load', function () { window.OpenAPIStart(" + string(encoded) + "); });\n"
	page := "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>" + html.EscapeString(cfg.Title) + "</title><link rel=\"stylesheet\" href=\"./swagger-ui.css\"><link rel=\"stylesheet\" href=\"./presentation.css\"><link rel=\"icon\" href=\"./favicon-32x32.png\"></head><body><div id=\"swagger-ui\"></div><script src=\"./swagger-ui-bundle.js\"></script><script src=\"./swagger-ui-standalone-preset.js\"></script><script src=\"./display-names.js\"></script><script src=\"./native-examples.js\"></script><script src=\"./native-compatibility.js\"></script><script src=\"./startup.js\"></script><script src=\"./config.js\"></script></body></html>"
	ui.resources["presentation.css"] = resource([]byte(presentationStyles), "text/css; charset=utf-8", "no-cache")
	ui.resources["display-names.js"] = resource([]byte(displayNames), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["native-examples.js"] = resource([]byte(nativeExamples), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["native-compatibility.js"] = resource([]byte(nativeCompatibility), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["startup.js"] = resource([]byte(startupScript), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["config.js"] = resource([]byte(script), "text/javascript; charset=utf-8", "no-cache")
	ui.resources["index.html"] = resource([]byte(page), "text/html; charset=utf-8", "no-cache")
	return ui, nil
}

// Compute deterministic ETags for cached resource responses.
func resource(raw []byte, contentType, cacheControl string) Resource {
	sum := sha256.Sum256(raw)
	return Resource{body: string(raw), contentType: contentType, etag: "\"" + hex.EncodeToString(sum[:]) + "\"", cacheControl: cacheControl}
}

// Read one named resource and reject traversal and encoded bypasses.
func (u *UI) Resource(name string) (Resource, error) {
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || strings.Contains(name, "%") || path.Clean(name) != name {
		return Resource{}, fmt.Errorf("openapi.ui.path: invalid resource path")
	}
	r, ok := u.resources[name]
	if !ok {
		return Resource{}, fs.ErrNotExist
	}
	return r, nil
}

// Return sorted resource names for transport-neutral preloading.
func (u *UI) Names() []string {
	out := make([]string, 0, len(u.resources))
	for name := range u.resources {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Restrict all specification entry points to same-origin paths, including checks against encoded parent traversal.
func localSpecURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || value == "" || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Contains(parsed.Path, "..") || strings.HasPrefix(value, "//") || strings.ContainsAny(parsed.Path, "\\\r\n") {
		return fmt.Errorf("openapi.ui.specURL: only local specification URLs are allowed")
	}
	return nil
}
