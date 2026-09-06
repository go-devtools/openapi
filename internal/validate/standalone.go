package validate

import (
	"net/url"
	"strings"
)

// Rewrite projection component references within actual resource scopes, then run shared offline checks.
func Standalone(raw []byte, componentNames []string, options Options) ([]byte, []Issue) {
	options.schemaOnly = true
	set, issues := prepareSchemaResources(raw, options, false)
	if len(issues) > 0 {
		return nil, issues
	}
	g := set.graph
	root := g.nodes["#"]
	if root == nil || root.role != "schema" {
		g.add("resource.type", "#", "standalone export requires a Schema root")
		return nil, g.issues
	}
	components := map[string]bool{}
	for _, name := range componentNames {
		components[name] = true
	}
	for i, use := range g.uses {
		if g.budget.exceeded {
			break
		}
		owner := g.nodes[use.owner]
		if owner == nil || owner.role != "schema" || owner.document != "#" {
			continue
		}
		if !g.spend(len(use.base), len(use.value)) {
			break
		}
		uri, err := absoluteReference(use.base, use.value)
		if err != nil {
			continue
		}
		const prefix = "/components/schemas/"
		if g.resources[resourceURI(uri)] != root || !strings.HasPrefix(uri.Fragment, prefix) {
			continue
		}
		if !g.spend(len(uri.Fragment), len(root.base)) {
			break
		}
		encodedName := strings.SplitN(strings.TrimPrefix(uri.Fragment, prefix), "/", 2)[0]
		name := strings.ReplaceAll(strings.ReplaceAll(encodedName, "~1", "/"), "~0", "~")
		if !components[name] {
			g.add("ref.missing", use.path, "referenced projection component does not exist: "+name)
			continue
		}
		pointer := "/$defs/" + strings.TrimPrefix(uri.Fragment, prefix)
		path, code := g.pointerPath(root, pointer)
		if code != "" {
			g.add(code, use.path, "invalid projection component pointer")
			continue
		}
		target := g.nodes[path]
		if target == nil || target.role != "schema" {
			g.add("ref.type", use.path, "component reference must target a Schema")
			continue
		}
		resource := g.resources[target.base]
		canonical := &url.URL{Fragment: strings.TrimPrefix(path, resource.path)}
		if target.base != owner.base {
			canonical, err = url.Parse(target.base)
			if err != nil {
				g.add("ref.uri", use.path, "invalid component resource identity")
				continue
			}
			canonical.Fragment = strings.TrimPrefix(path, resource.path)
			if resource == root {
				root.value.(map[string]any)["$id"] = root.base
			}
		}
		replacement := canonical.String()
		if replacement == "" {
			replacement = "#"
		}
		if !g.spend(len(replacement)) {
			break
		}
		key := "$ref"
		if strings.HasSuffix(use.path, "/$dynamicRef") {
			key = "$dynamicRef"
		}
		owner.value.(map[string]any)[key] = replacement
		g.uses[i].value = replacement
	}
	if len(g.issues) > 0 {
		return nil, sortedIssues(g.issues)
	}
	return set.exportStandalone(root, options)
}
