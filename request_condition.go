package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/openapi-golang/openapi/spec"
)

// Describe finite method and media decisions; an empty inclusion set leaves that dimension unrestricted.
type RequestCondition struct {
	Methods          []string `json:"methods,omitempty"`
	ExceptMethods    []string `json:"exceptMethods,omitempty"`
	MediaTypes       []string `json:"mediaTypes,omitempty"`
	ExceptMediaTypes []string `json:"exceptMediaTypes,omitempty"`
}

// Store a projected conditional contract without business functions or analysis-time type objects.
type OperationVariant struct {
	When        RequestCondition `json:"when"`
	Operation   spec.Operation   `json:"operation"`
	Diagnostics []Diagnostic     `json:"diagnostics,omitempty"`
	Facts       []Source         `json:"facts,omitempty"`
}

// Identify the runtime capability required for conditional Bundle selection.
const RequestConditionsCapability = "request-conditions-v1"

// Report whether a condition applies to every request without changing caller-owned sets.
func (c RequestCondition) Unconditional() bool {
	return len(c.Methods)+len(c.ExceptMethods)+len(c.MediaTypes)+len(c.ExceptMediaTypes) == 0
}

// Normalize finite sets, reject invalid conditions, and report empty intersections.
func (c RequestCondition) Intersect(other RequestCondition) (RequestCondition, bool, error) {
	a, ok, err := normalizeCondition(c)
	if err != nil || !ok {
		return a, ok, err
	}
	b, ok, err := normalizeCondition(other)
	if err != nil || !ok {
		return b, ok, err
	}
	methods, exceptMethods, ok := intersectDomain(a.Methods, a.ExceptMethods, b.Methods, b.ExceptMethods)
	if !ok {
		return RequestCondition{}, false, nil
	}
	media, exceptMedia, ok := intersectDomain(a.MediaTypes, a.ExceptMediaTypes, b.MediaTypes, b.ExceptMediaTypes)
	if !ok {
		return RequestCondition{}, false, nil
	}
	return RequestCondition{Methods: methods, ExceptMethods: exceptMethods, MediaTypes: media, ExceptMediaTypes: exceptMedia}, true, nil
}

// Match exact HTTP tokens; an empty media string denotes an absent Content-Type.
func validConditionToken(value string) bool {
	if value == "" {
		return false
	}
	for _, b := range []byte(value) {
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))) {
			return false
		}
	}
	return true
}

// Copy and sort sets, validating method and media names without implicit case rewriting.
func conditionSet(values []string, media bool) ([]string, error) {
	if len(values) > 256 {
		return nil, fmt.Errorf("openapi.condition.budget: condition set exceeds the limit")
	}
	set := map[string]bool{}
	for _, value := range values {
		valid := validConditionToken(value)
		if media {
			parts := strings.Split(value, "/")
			valid = value == "" || len(parts) == 2 && validConditionToken(parts[0]) && validConditionToken(parts[1]) && !strings.Contains(value, "*")
		}
		if !valid {
			return nil, fmt.Errorf("openapi.condition.invalid: invalid condition value %q", value)
		}
		set[value] = true
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

// Remove duplicates and exclusions without mistaking an empty reachable set for an unconditional condition.
func normalizeCondition(c RequestCondition) (RequestCondition, bool, error) {
	lists := []*[]string{&c.Methods, &c.ExceptMethods, &c.MediaTypes, &c.ExceptMediaTypes}
	for i, list := range lists {
		values, err := conditionSet(*list, i >= 2)
		if err != nil {
			return c, false, err
		}
		*list = values
	}
	var ok bool
	c.Methods, c.ExceptMethods, ok = intersectDomain(c.Methods, c.ExceptMethods, nil, nil)
	if !ok {
		return c, false, nil
	}
	c.MediaTypes, c.ExceptMediaTypes, ok = intersectDomain(c.MediaTypes, c.ExceptMediaTypes, nil, nil)
	return c, ok, nil
}

// Intersect inclusion/exclusion domains and return independently owned slices.
func intersectDomain(a, ax, b, bx []string) ([]string, []string, bool) {
	excluded := map[string]bool{}
	for _, v := range ax {
		excluded[v] = true
	}
	for _, v := range bx {
		excluded[v] = true
	}
	var allowed []string
	finite := len(a) > 0 || len(b) > 0
	if len(a) > 0 {
		for _, v := range a {
			if len(b) == 0 || stringMember(b, v) {
				if !excluded[v] {
					allowed = append(allowed, v)
				}
			}
		}
	} else {
		for _, v := range b {
			if !excluded[v] {
				allowed = append(allowed, v)
			}
		}
	}
	if finite {
		return allowed, nil, len(allowed) > 0
	}
	exceptions := make([]string, 0, len(excluded))
	for v := range excluded {
		exceptions = append(exceptions, v)
	}
	sort.Strings(exceptions)
	return nil, exceptions, true
}

// Check membership in a small finite set without relying on external sorting.
func stringMember(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// Decide whether a request dimension satisfies inclusion and exclusion sets.
func conditionAllows(included, excluded []string, value string) bool {
	return (len(included) == 0 || stringMember(included, value)) && !stringMember(excluded, value)
}

// Select static contracts using the method and explicit media configuration, retaining condition provenance.
func linkConditionalTemplate(template Template, route Route) (spec.Operation, []Diagnostic, []Source, error) {
	op := copyJSON(template.Operation)
	diagnostics := append([]Diagnostic(nil), template.Diagnostics...)
	facts := append([]Source(nil), template.Facts...)
	if len(template.Variants) == 0 {
		return op, diagnostics, facts, nil
	}
	media, err := conditionSet(route.RequestMediaTypes, true)
	if err != nil {
		return op, diagnostics, facts, err
	}
	selected := map[int]bool{}
	if len(media) > 0 {
		when := RequestCondition{Methods: []string{route.Method}, MediaTypes: append([]string(nil), media...)}
		facts = append(facts, Source{Kind: "declared", Rule: "openapi.Route.RequestMediaTypes", When: &when})
	}
	if len(media) == 0 {
		for i, variant := range template.Variants {
			if conditionAllows(variant.When.Methods, variant.When.ExceptMethods, route.Method) {
				if len(variant.When.MediaTypes)+len(variant.When.ExceptMediaTypes) > 0 {
					return op, diagnostics, facts, fmt.Errorf("openapi.condition.media: %s %s requires an explicit request media type", route.Method, route.Path)
				}
				selected[i] = true
			}
		}
	} else {
		for _, kind := range media {
			matched := false
			for i, variant := range template.Variants {
				when := variant.When
				if conditionAllows(when.Methods, when.ExceptMethods, route.Method) && conditionAllows(when.MediaTypes, when.ExceptMediaTypes, kind) {
					selected[i] = true
					matched = true
				}
			}
			if !matched {
				return op, diagnostics, facts, fmt.Errorf("openapi.condition.uncovered: %s %s has no contract for media type %q", route.Method, route.Path, kind)
			}
		}
	}
	if len(selected) == 0 {
		return op, diagnostics, facts, fmt.Errorf("openapi.condition.uncovered: %s %s has no applicable contract", route.Method, route.Path)
	}
	var parameters []byte
	var bodyRequired *bool
	for i, variant := range template.Variants {
		if selected[i] {
			// Media-dependent presence cannot be projected losslessly into one body-required boolean.
			required := variant.Operation.RequestBody != nil && variant.Operation.RequestBody.Value != nil && variant.Operation.RequestBody.Value.Required.Value
			if bodyRequired != nil && *bodyRequired != required {
				return op, diagnostics, facts, fmt.Errorf("openapi.condition.ambiguous: request body required differs across conditions")
			}
			bodyRequired = &required
			parameterJSON, _ := json.Marshal(variant.Operation.Parameters)
			if parameters != nil && !bytes.Equal(parameters, parameterJSON) {
				return op, diagnostics, facts, fmt.Errorf("openapi.condition.ambiguous: parameter contracts from different conditions cannot be merged without loss")
			}
			parameters = parameterJSON
			if err := mergeConditionalOperation(&op, variant.Operation); err != nil {
				return op, diagnostics, facts, err
			}
			for _, diagnostic := range variant.Diagnostics {
				source := diagnostic.Source
				when := copyJSON(variant.When)
				source.When = &when
				diagnostic.Source = source
				diagnostics = append(diagnostics, diagnostic)
			}
			for _, source := range variant.Facts {
				when := copyJSON(variant.When)
				source.When = &when
				facts = append(facts, source)
			}
		}
	}
	return op, diagnostics, facts, nil
}

// Merge compatible metadata fields and diagnose conflicts instead of overwriting by order.
func mergeConditionMetadata(target, source any, ignored ...string) error {
	leftRaw, err := json.Marshal(target)
	if err != nil {
		return err
	}
	rightRaw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	var left, right map[string]json.RawMessage
	if err = json.Unmarshal(leftRaw, &left); err != nil {
		return err
	}
	if err = json.Unmarshal(rightRaw, &right); err != nil {
		return err
	}
	for key, value := range right {
		if stringMember(ignored, key) {
			continue
		}
		if old, ok := left[key]; ok && !bytes.Equal(old, value) {
			return fmt.Errorf("openapi.condition.ambiguous: %s differs across conditional branches", key)
		}
		left[key] = value
	}
	merged, err := json.Marshal(left)
	if err != nil {
		return err
	}
	return json.Unmarshal(merged, target)
}

// Merge conditional request and response representations while requiring compatible metadata elsewhere.
func mergeConditionalOperation(target *spec.Operation, source spec.Operation) error {
	if err := mergeConditionMetadata(target, source, "requestBody", "responses"); err != nil {
		return err
	}
	if source.RequestBody != nil {
		if target.RequestBody == nil {
			target.RequestBody = copyJSON(source.RequestBody)
		} else {
			if target.RequestBody.Value == nil || source.RequestBody.Value == nil {
				if !jsonSame(target.RequestBody, source.RequestBody) {
					return fmt.Errorf("openapi.condition.ambiguous: request body references differ")
				}
			} else {
				a, b := target.RequestBody.Value, source.RequestBody.Value
				if err := mergeConditionMetadata(a, b, "content"); err != nil {
					return err
				}
				if err := mergeConditionContent(&a.Content, b.Content); err != nil {
					return err
				}
			}
		}
	}
	if target.Responses == nil {
		target.Responses = map[string]spec.RefOr[spec.Response]{}
	}
	for status, incoming := range source.Responses {
		existing, found := target.Responses[status]
		if !found {
			target.Responses[status] = copyJSON(incoming)
			continue
		}
		if existing.Value == nil || incoming.Value == nil {
			if !jsonSame(existing, incoming) {
				return fmt.Errorf("openapi.condition.ambiguous: response references differ")
			}
			continue
		}
		a, b := existing.Value, incoming.Value
		if err := mergeConditionMetadata(a, b, "content", "headers"); err != nil {
			return err
		}
		if err := mergeConditionContent(&a.Content, b.Content); err != nil {
			return err
		}
		if a.Headers == nil && len(b.Headers) > 0 {
			a.Headers = map[string]spec.RefOr[spec.Header]{}
		}
		for name, header := range b.Headers {
			old, present := a.Headers[name]
			if !present {
				a.Headers[name] = copyJSON(header)
				continue
			}
			if old.Value == nil || header.Value == nil {
				if !jsonSame(old, header) {
					return fmt.Errorf("openapi.condition.ambiguous: response header references differ")
				}
				continue
			}
			if err := mergeConditionMetadata(old.Value, header.Value, "schema"); err != nil {
				return err
			}
			old.Value.Schema = conditionalSchemaUnion(old.Value.Schema, header.Value.Schema)
		}
	}
	return nil
}

// Compare deterministic JSON encodings without relying on pointer identity.
func jsonSame(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && bytes.Equal(left, right)
}

// Merge distinct media types and use anyOf for overlapping schema alternatives.
func mergeConditionContent(target *map[string]spec.RefOr[spec.MediaType], source map[string]spec.RefOr[spec.MediaType]) error {
	if *target == nil && len(source) > 0 {
		*target = map[string]spec.RefOr[spec.MediaType]{}
	}
	for kind, media := range source {
		old, ok := (*target)[kind]
		if !ok {
			(*target)[kind] = copyJSON(media)
			continue
		}
		if old.Value == nil || media.Value == nil {
			if !jsonSame(old, media) {
				return fmt.Errorf("openapi.condition.ambiguous: media type references differ")
			}
			continue
		}
		// Conditions constraining complete bodies versus stream items must not degrade into unconstrained media objects.
		a, b := old.Value, media.Value
		if (a.ItemSchema != nil && a.Schema == nil && b.Schema != nil && b.ItemSchema == nil) ||
			(b.ItemSchema != nil && b.Schema == nil && a.Schema != nil && a.ItemSchema == nil) {
			return fmt.Errorf("openapi.condition.ambiguous: whole-body and item-wise responses use different framing across conditions")
		}
		if err := mergeConditionMetadata(old.Value, media.Value, "schema", "itemSchema", "encoding"); err != nil {
			return err
		}
		// Merge encodings by field; distinct fields can coexist, while shared fields must keep the same representation.
		keys := make([]string, 0, len(media.Value.Encoding))
		for key := range media.Value.Encoding {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			encoding := media.Value.Encoding[key]
			if previous, exists := old.Value.Encoding[key]; exists && !jsonSame(previous, encoding) {
				return fmt.Errorf("openapi.condition.ambiguous: request field %s encoding differs", key)
			}
			if old.Value.Encoding == nil {
				old.Value.Encoding = map[string]spec.Encoding{}
			}
			old.Value.Encoding[key] = copyJSON(encoding)
		}
		old.Value.Schema = conditionalSchemaUnion(old.Value.Schema, media.Value.Schema)
		if old.Value.ItemSchema != nil || media.Value.ItemSchema != nil {
			old.Value.ItemSchema = conditionalSchemaUnion(old.Value.ItemSchema, media.Value.ItemSchema)
		}
	}
	return nil
}

// Merge conditional schemas into detached copies without narrowing an unconstrained alternative.
func conditionalSchemaUnion(a, b *spec.Schema) *spec.Schema {
	if a == nil || b == nil {
		return nil
	}
	if jsonSame(a, b) {
		return copyJSON(a)
	}

	return &spec.Schema{SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{copyJSON(a), copyJSON(b)}}}
}
