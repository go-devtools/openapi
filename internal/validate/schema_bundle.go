package validate

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Carries resources and the selected location for an independent instance validator; it does not validate instances.
type SchemaBundle struct {
	Value     any
	URI       string
	Pointer   string
	Resources map[string]any
}

// Moves OpenAPI schemas into standard $defs while preserving resource identifiers and dynamic anchors.
func BundleSchemas(raw []byte, pointer string, options Options) (*SchemaBundle, []Issue) {
	set, issues := prepareSchemaResources(raw, options, false)
	if len(issues) > 0 {
		return nil, issues
	}
	g := set.graph
	if !g.spend(len(pointer)) {
		return nil, g.issues
	}
	if pointer != "" && !strings.HasPrefix(pointer, "/") {
		g.add("ref.pointer", "#", "use an empty pointer or an absolute JSON Pointer")
		return nil, g.issues
	}
	selectedPath, code := "#", ""
	if pointer != "" {
		selectedPath, code = g.pointerPath(g.nodes["#"], pointer)
	}
	if code != "" {
		g.add(code, "#"+pointer, "selected JSON Pointer does not exist or is invalid")
		return nil, g.issues
	}
	selected := g.nodes[selectedPath]
	if selected == nil || selected.role != "schema" {
		g.add("ref.type", "#"+pointer, "select a standard Schema location, not an OpenAPI object or example data")
		return nil, g.issues
	}
	locations := map[string]string{}
	definitions := map[string]any{}
	resourceValues := map[string]any{}
	paths := make([]string, 0, len(g.nodes))
	for path := range g.nodes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for i, entry := range set.documents {
		if g.budget.exceeded {
			return nil, sortedIssues(append(issues, g.issues...))
		}
		key := fmt.Sprintf("resource%d", i)
		location := g.location("/$defs/", key)
		locations[entry.path] = location
		root := g.nodes[entry.path]
		if entry.role == "schema" {
			definitions[key] = entry.value
			for _, path := range paths {
				if g.budget.exceeded {
					return nil, sortedIssues(append(issues, g.issues...))
				}
				if path == entry.path || strings.HasPrefix(path, entry.path+"/") {
					locations[path] = g.location(location, strings.TrimPrefix(path, entry.path))
				}
			}
			c := checker{graph: g}
			c.walk(entry.value, entry.path, "schema", 0)
			issues = append(issues, c.issues...)
		} else {
			schemas := map[string]any{}
			resource := map[string]any{"$id": root.base, "$defs": schemas}
			if object, ok := entry.value.(map[string]any); ok {
				if dialect, exists := object["jsonSchemaDialect"]; exists {
					resource["$schema"] = dialect
				}
			}
			definitions[key] = resource
			rootCount := 0
			for _, path := range paths {
				if g.budget.exceeded {
					return nil, sortedIssues(append(issues, g.issues...))
				}
				node := g.nodes[path]
				if node.document != entry.path || node.role != "schema" {
					continue
				}
				ancestor := ""
				parent := path
				for {
					index := strings.LastIndexByte(parent, '/')
					if index < len(entry.path) {
						break
					}
					parent = parent[:index]
					if node := g.nodes[parent]; node != nil && node.role == "schema" {
						ancestor = parent
						break
					}
				}
				if ancestor != "" {
					locations[path] = g.location(locations[ancestor], strings.TrimPrefix(path, ancestor))
					continue
				}
				name := fmt.Sprintf("schema%d", rootCount)
				rootCount++
				locations[path] = g.location(location, "/$defs/", name)
				schemas[name] = node.value
				c := checker{graph: g}
				c.walk(node.value, path, "schema", 0)
				issues = append(issues, c.issues...)
			}
		}
		// Each input document already has its own base; allOf preserves a boolean root's validation result.
		if object, ok := definitions[key].(map[string]any); ok {
			object["$id"] = root.base
		} else {
			definitions[key] = map[string]any{"$id": root.base, "allOf": []any{definitions[key]}}
		}
		resourceValues[entry.path] = definitions[key]
	}
	if g.budget.exceeded {
		return nil, sortedIssues(append(issues, g.issues...))
	}
	if len(issues) > 0 {
		return nil, sortedIssues(issues)
	}
	// Resolve existing identifiers without visiting business data in const, default, examples, or extensions.
	for _, path := range paths {
		if g.budget.exceeded {
			return nil, sortedIssues(append(issues, g.issues...))
		}
		node := g.nodes[path]
		if node.role == "schema" {
			if object, ok := node.value.(map[string]any); ok && has(object, "$id") {
				object["$id"] = node.base
			}
		}
	}
	value := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": definitions}
	normalized := byteBudget{remaining: g.maxNormalizedBytes}
	if !normalized.jsonValue(value) {
		g.add("budget", "#", "normalized Schema exceeds MaxNormalizedBytes")
		return nil, g.issues
	}
	for _, use := range g.uses {
		if g.budget.exceeded {
			break
		}
		owner := g.nodes[use.owner]
		if owner == nil || owner.role != "schema" {
			continue
		}
		key := ""
		if strings.HasSuffix(use.path, "/$ref") {
			key = "$ref"
		} else if strings.HasSuffix(use.path, "/$dynamicRef") {
			key = "$dynamicRef"
		}
		if key == "" || use.role != "schema" {
			continue
		}
		target, code, message := g.target(use)
		if code != "" {
			g.add(code, use.path, message)
			continue
		}
		uri, err := absoluteReference(use.base, use.value)
		if err != nil {
			g.add("ref.uri", use.path, err.Error())
			continue
		}
		resource := g.resources[resourceURI(uri)]
		canonical, err := url.Parse(resource.base)
		if err != nil {
			g.add("ref.uri", use.path, err.Error())
			continue
		}
		canonical.Fragment = uri.Fragment
		if strings.HasPrefix(uri.Fragment, "/") {
			canonical.Fragment = strings.TrimPrefix(locations[target.path], locations[resource.path])
		}
		// Keep anchor fragments instead of replacing them with JSON Pointers, which would make dynamicRef static.
		if !normalized.replaceString(owner.value.(map[string]any), key, canonical.String(), g.maxNormalizedBytes) {
			g.add("budget", "#", "normalized Schema exceeds MaxNormalizedBytes")
			return nil, g.issues
		}
	}
	if len(g.issues) > 0 {
		return nil, sortedIssues(g.issues)
	}
	resources := map[string]any{}
	for uri, node := range g.resources {
		value := resourceValues[node.path]
		if value == nil {
			value = node.value
		}
		resources[uri] = value
	}
	uri := "urn:openapi:contract:bundle"
	for g.resources[uri] != nil {
		uri += ":container"
	}
	return &SchemaBundle{Value: value, URI: uri, Pointer: locations[selected.path], Resources: resources}, nil
}
