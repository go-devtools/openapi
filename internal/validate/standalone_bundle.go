package validate

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// 保留各资源的身份与方言，在根定义中嵌入显式依赖并规范化检索别名。
// Preserve resource identities and dialects, embed explicit dependencies, and normalize retrieval aliases.
func (set *resourceSet) exportStandalone(root *referenceNode, options Options) ([]byte, []Issue) {
	g := set.graph
	for _, entry := range set.documents {
		if entry.role != "schema" {
			g.add("resource.type", entry.path, "独立导出的预载内容必须是 JSON Schema")
			return nil, g.issues
		}
	}
	for _, path := range sortedKeys(g.nodes) {
		node := g.nodes[path]
		if node.role == "schema" {
			if object, ok := node.value.(map[string]any); ok && has(object, "$id") {
				object["$id"] = node.base
			}
		}
	}
	if issues := set.check(); len(issues) > 0 {
		return nil, issues
	}
	object, ok := root.value.(map[string]any)
	if !ok {
		object = map[string]any{"allOf": []any{root.value}}
		root.value = object
	}
	if len(set.documents) > 1 || options.BaseURI != "" {
		object["$id"] = root.base
	}
	definitions, _ := object["$defs"].(map[string]any)
	if definitions == nil {
		definitions = map[string]any{}
		object["$defs"] = definitions
	}
	next := 0
	for _, entry := range set.documents[1:] {
		node := g.nodes[entry.path]
		value, ok := entry.value.(map[string]any)
		if !ok {
			value = map[string]any{"allOf": []any{entry.value}}
		}
		value["$id"] = node.base
		if !has(value, "$schema") {
			value["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		}
		key := ""
		for {
			key = fmt.Sprintf("resource%d", next)
			next++
			if _, exists := definitions[key]; !exists {
				break
			}
		}
		if !g.spend(len(key)) {
			return nil, g.issues
		}
		definitions[key] = value
	}
	budget := byteBudget{remaining: g.maxNormalizedBytes}
	if !budget.jsonValue(object) {
		g.add("budget", "#", "导出 Schema 超过 MaxNormalizedBytes")
		return nil, g.issues
	}
	for _, use := range g.uses {
		owner := g.nodes[use.owner]
		if owner == nil || owner.role != "schema" {
			continue
		}
		if !g.spend(len(use.base), len(use.value)) {
			return nil, g.issues
		}
		uri, err := absoluteReference(use.base, use.value)
		if err != nil {
			g.add("ref.uri", use.path, err.Error())
			return nil, g.issues
		}
		resource := g.resources[resourceURI(uri)]
		if resource == nil {
			g.add("external.denied", use.path, "缺少导出依赖资源")
			return nil, g.issues
		}
		if strings.HasPrefix(use.value, "#") && resource.base == owner.base {
			continue
		}
		if !g.spend(len(resource.base), len(uri.Fragment)) {
			return nil, g.issues
		}
		canonical, err := url.Parse(resource.base)
		if err != nil {
			g.add("ref.uri", use.path, err.Error())
			return nil, g.issues
		}
		canonical.Fragment = uri.Fragment
		if resource == root && !has(object, "$id") {
			object["$id"] = root.base
			budget = byteBudget{remaining: g.maxNormalizedBytes}
			if !budget.jsonValue(object) {
				g.add("budget", "#", "导出 Schema 超过 MaxNormalizedBytes")
				return nil, g.issues
			}
		}
		key := "$ref"
		if strings.HasSuffix(use.path, "/$dynamicRef") {
			key = "$dynamicRef"
		}
		if !budget.replaceString(owner.value.(map[string]any), key, canonical.String(), g.maxNormalizedBytes) {
			g.add("budget", "#", "导出 Schema 超过 MaxNormalizedBytes")
			return nil, g.issues
		}
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		g.add("json", "#", err.Error())
		return nil, g.issues
	}
	// 对最终单文档重新建图，证明它不再依赖调用方另行提供资源。
	// Re-index the final document to prove it no longer relies on separately supplied resources.
	outputOptions := options
	outputOptions.Resources = nil
	outputOptions.ExampleResources = nil
	outputOptions.MaxBytes = g.maxNormalizedBytes
	if issues := CheckSchemaWithOptions(encoded, outputOptions); len(issues) > 0 {
		return nil, issues
	}
	return encoded, nil
}
