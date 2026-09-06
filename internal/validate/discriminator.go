package validate

import (
	"strconv"
	"strings"
)

// Track reverse same-instance relationships; an allOf edge establishes schema inheritance.
type schemaParent struct {
	node  *referenceNode
	allOf bool
}

// Share resolved references and bounded inheritance traversal across discriminator checks.
type discriminatorContext struct {
	checker *checker
	targets map[string]*referenceNode
	parents map[string][]schemaParent
}

// Check dispatch context after reference indexing without changing JSON Schema instance validation.
func (c *checker) checkDiscriminators() {
	g := c.graph
	if g == nil || g.schemaOnly || c.stopped() {
		return
	}
	var discriminators []*referenceNode
	for _, path := range sortedKeys(g.nodes) {
		node := g.nodes[path]
		if node.role == "discriminator" {
			discriminators = append(discriminators, node)
		}
	}
	if len(discriminators) == 0 {
		return
	}
	context := discriminatorContext{checker: c, targets: map[string]*referenceNode{}, parents: map[string][]schemaParent{}}
	for _, use := range g.uses {
		if c.stopped() {
			return
		}
		if use.role != "schema" {
			continue
		}
		target, code, _ := g.target(use)
		if code == "" && target != nil && g.spend(64) {
			context.targets[use.path] = target
		}
	}
	for _, path := range sortedKeys(g.nodes) {
		if c.stopped() {
			return
		}
		node := g.nodes[path]
		if node.role != "schema" {
			continue
		}
		if target := context.targets[path+"/$ref"]; target != nil {
			context.parent(target, node, false)
		}
		object, _ := node.value.(map[string]any)
		parts, _ := object["allOf"].([]any)
		for i := range parts {
			if child := g.nodes[path+"/allOf/"+strconv.Itoa(i)]; child != nil {
				context.parent(child, node, true)
			}
		}
	}
	for _, node := range discriminators {
		if c.stopped() {
			return
		}
		context.check(node)
	}
}

// Charge each graph edge before retaining it in the reverse index.
func (d *discriminatorContext) parent(child, parent *referenceNode, allOf bool) {
	if d.checker.graph.spend(64) {
		d.parents[child.path] = append(d.parents[child.path], schemaParent{parent, allOf})
	}
}

// Find transitive children through references and at least one allOf edge, including cyclic graphs.
func (d *discriminatorContext) descendants(node *referenceNode) map[string]bool {
	found := map[string]bool{}
	visited := map[schemaParent]bool{}
	queue := []schemaParent{{node: node}}
	for i := 0; i < len(queue); i++ {
		state := queue[i]
		if visited[state] {
			continue
		}
		if !d.checker.graph.spend(64) {
			return found
		}
		visited[state] = true
		if state.allOf && state.node.path != node.path {
			found[state.node.path] = true
		}
		for _, parent := range d.parents[state.node.path] {
			if !d.checker.graph.spend(32) {
				return found
			}
			queue = append(queue, schemaParent{node: parent.node, allOf: state.allOf || parent.allOf})
		}
	}
	return found
}

// Follow only ordinary references; dynamic targets cannot be certified from their initial target alone.
func (d *discriminatorContext) referenceChain(node *referenceNode) map[string]bool {
	chain := map[string]bool{}
	for node != nil && !chain[node.path] {
		if !d.checker.graph.spend(32) {
			break
		}
		chain[node.path] = true
		node = d.targets[node.path+"/$ref"]
	}
	return chain
}

// Compare aliases by their resolved identity while retaining the original candidate list.
func (d *discriminatorContext) sameSchema(a, b *referenceNode) bool {
	left := d.referenceChain(a)
	for path := range d.referenceChain(b) {
		if left[path] {
			return true
		}
	}
	return false
}

// Prove explicit required constraints through references, conjunctions and every alternative branch.
func (d *discriminatorContext) requires(node *referenceNode, property string, memo map[string]bool, visiting map[string]bool) bool {
	if node == nil || visiting[node.path] || d.checker.stopped() {
		return false
	}
	if result, known := memo[node.path]; known {
		return result
	}
	if !d.checker.graph.spend(64) {
		return false
	}
	visiting[node.path] = true
	defer delete(visiting, node.path)
	object, _ := node.value.(map[string]any)
	required, _ := object["required"].([]any)
	for _, name := range required {
		if name == property {
			memo[node.path] = true
			return true
		}
	}
	if d.requires(d.targets[node.path+"/$ref"], property, memo, visiting) {
		memo[node.path] = true
		return true
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		children, _ := object[keyword].([]any)
		all := len(children) > 0
		for i := range children {
			if d.checker.stopped() {
				return false
			}
			child := d.checker.graph.nodes[node.path+"/"+keyword+"/"+strconv.Itoa(i)]
			required := d.requires(child, property, memo, visiting)
			if keyword == "allOf" && required {
				memo[node.path] = true
				return true
			}
			all = all && required
		}
		if keyword != "allOf" && all {
			memo[node.path] = true
			return true
		}
	}
	if has(object, "if") && has(object, "then") && has(object, "else") &&
		d.requires(d.checker.graph.nodes[node.path+"/then"], property, memo, visiting) &&
		d.requires(d.checker.graph.nodes[node.path+"/else"], property, memo, visiting) {
		memo[node.path] = true
		return true
	}
	memo[node.path] = false
	return false
}

// Enforce candidate membership and a usable omission fallback without selecting an instance branch.
func (d *discriminatorContext) check(node *referenceNode) {
	c := d.checker
	owner := c.graph.nodes[strings.TrimSuffix(node.path, "/discriminator")]
	object, ok := node.value.(map[string]any)
	property, propertyOK := object["propertyName"].(string)
	if owner == nil || !ok || !propertyOK {
		return
	}
	schema, _ := owner.value.(map[string]any)
	var candidates []*referenceNode
	union := false
	for _, keyword := range []string{"oneOf", "anyOf"} {
		if has(schema, keyword) {
			union = true
		}
		entries, _ := schema[keyword].([]any)
		for i := range entries {
			if !c.graph.spend(16) {
				return
			}
			if candidate := c.graph.nodes[owner.path+"/"+keyword+"/"+strconv.Itoa(i)]; candidate != nil {
				candidates = append(candidates, candidate)
			}
		}
	}
	descendants := map[string]bool{}
	if !union {
		descendants = d.descendants(owner)
	}
	if !union && !has(schema, "allOf") && len(descendants) == 0 {
		c.add("discriminator.context", node.path, "discriminator requires an adjacent oneOf/anyOf/allOf or a parent referenced through allOf")
	}
	required := d.requires(owner, property, map[string]bool{}, map[string]bool{})
	if !required && !has(object, "defaultMapping") {
		c.add("discriminator.default.required", node.path+"/defaultMapping", "discriminating property is not provably required; provide defaultMapping or an explicit required constraint")
	}
	mapping, _ := object["mapping"].(map[string]any)
	paths := make([]string, 0, len(mapping)+1)
	for _, key := range sortedKeys(mapping) {
		paths = append(paths, node.path+"/mapping/"+escape(key))
	}
	if has(object, "defaultMapping") {
		paths = append(paths, node.path+"/defaultMapping")
	}
	for _, path := range paths {
		if c.stopped() {
			return
		}
		target := d.targets[path]
		if target == nil {
			continue
		}
		listed := false
		if union {
			for _, candidate := range candidates {
				if d.sameSchema(candidate, target) {
					listed = true
					break
				}
			}
		} else {
			for identity := range d.referenceChain(target) {
				if descendants[identity] {
					listed = true
					break
				}
			}
		}
		if !listed {
			c.add("discriminator.target", path, "mapped schema must be an explicitly listed union candidate or an allOf descendant of the discriminator parent")
		}
		if path == node.path+"/defaultMapping" && !required && d.requires(target, property, map[string]bool{}, map[string]bool{}) {
			c.add("discriminator.default.optional", path, "defaultMapping requires the discriminating property and cannot validate an object where it is omitted")
		}
	}
}
