package compiler

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"sort"

	"github.com/openapi-golang/openapi"
)

// Retain summaries only for immutable parameter contexts within one candidate's analysis.
type helperSummaryCache struct {
	entries map[string]*helperSummary
	types   map[types.Type]int
	objects map[types.Object]int
	bytes   int
	hits    int
}

// Store correlated return tuples, effect deltas, and their original analysis costs.
type helperSummary struct {
	ready   bool
	results []helperSummaryResult
	calls   int
	depth   int
}

// Keep response transitions separate from caller-local cells and preexisting effects.
type helperSummaryResult struct {
	response flow
	effects  []Effect
	values   []Value
}

// Track deepest nested analysis even when an inner invocation reuses a summary.
type helperSummaryCapture struct {
	depth int
}

// Identify input facts without serializing Go objects or trusting their printable short names.
type summaryValueKey struct {
	Type, Object                                           int
	ConstantKind                                           constant.Kind
	Constant                                               string
	Fields                                                 map[string]summaryValueKey
	Nil, NonNil, DynamicNil, DynamicNonNil, Boxed, Unknown bool
}

// Keep header provenance in the key so replay cannot reuse another caller's source locations.
type summaryHeaderKey struct {
	Value  summaryValueKey
	Source openapi.Source
}

// Include portable effect fields while replacing analysis-only values and immutable rule handles.
type summaryEffectFields Effect

// Shadow nonportable effect fields with deterministic value and rule-presence data.
type summaryEffectKey struct {
	summaryEffectFields
	Payload         summaryValueKey
	Headers         map[string]summaryHeaderKey
	Codec           bool
	TransformSchema bool
}

// Normalize effect values without marshaling frontend functions or Go type objects.
func (cache *helperSummaryCache) effectKey(effect Effect, count *int) (summaryEffectKey, error) {
	value, err := cache.valueKey(effect.Payload, count, 0)
	if err != nil {
		return summaryEffectKey{}, err
	}
	headers, err := cache.headerKey(effect.Headers, count)
	if err != nil {
		return summaryEffectKey{}, err
	}
	return summaryEffectKey{summaryEffectFields: summaryEffectFields(effect), Payload: value, Headers: headers, Codec: effect.Codec != nil, TransformSchema: effect.TransformSchema != nil}, nil
}

// Preserve every response fact that can affect a helper's protocol transition.
type summaryResponseKey struct {
	Headers, Observed map[string]summaryHeaderKey
	Pending           any
	When              openapi.RequestCondition
	Committed         bool
	Status            string
	Writes            int
	BodyKind          EffectKind
	BodyMedia         string
}

// Iterate fact maps deterministically before assigning cache-local Go identity numbers.
func summaryMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

// Distinguish unsupported mutable contexts from actual resource-budget failures.
var errMutableSummary = errors.New("helper summary requires immutable parameter and result facts")

// Initialize a candidate-local cache so one unselected operation cannot consume another's budget.
func newHelperSummaryCache() *helperSummaryCache {
	return &helperSummaryCache{entries: map[string]*helperSummary{}, types: map[types.Type]int{}, objects: map[types.Object]int{}}
}

// Update every active outer summary's call-depth footprint.
func (a *analyzer) summaryDepth(depth int) {
	for _, capture := range a.summaryCaptures {
		if depth > capture.depth {
			capture.depth = depth
		}
	}
}

// Encode one immutable abstract value with bounded traversal and exact constant spelling.
func (c *helperSummaryCache) valueKey(value Value, count *int, depth int) (summaryValueKey, error) {
	if value.address != 0 || value.callable != nil {
		return summaryValueKey{}, errMutableSummary
	}
	*count += 1
	if *count > 4096 || depth > 64 {
		return summaryValueKey{}, fmt.Errorf("openapi.analysis.summary: parameter facts exceed the summary value budget")
	}
	out := summaryValueKey{Nil: value.Nil, NonNil: value.NonNil, DynamicNil: value.DynamicNil, DynamicNonNil: value.DynamicNonNil, Boxed: value.Boxed, Unknown: value.Unknown}
	if value.Type != nil {
		if !reflect.TypeOf(value.Type).Comparable() {
			return out, errMutableSummary
		}
		if c.types[value.Type] == 0 {
			c.types[value.Type] = len(c.types) + 1
		}
		out.Type = c.types[value.Type]
	}
	if value.Object != nil {
		if !reflect.TypeOf(value.Object).Comparable() {
			return out, errMutableSummary
		}
		if c.objects[value.Object] == 0 {
			c.objects[value.Object] = len(c.objects) + 1
		}
		out.Object = c.objects[value.Object]
	}
	if value.Constant != nil {
		out.ConstantKind = value.Constant.Kind()
		out.Constant = value.Constant.ExactString()
	}
	if value.Fields != nil {
		out.Fields = map[string]summaryValueKey{}
		for _, name := range summaryMapKeys(value.Fields) {
			field := value.Fields[name]
			key, err := c.valueKey(field, count, depth+1)
			if err != nil {
				return out, err
			}
			out.Fields[name] = key
		}
	}
	return out, nil
}

// Normalize all header facts, including the difference between absent and explicitly empty storage.
func (c *helperSummaryCache) headerKey(headers map[string]HeaderValue, count *int) (map[string]summaryHeaderKey, error) {
	if headers == nil {
		return nil, nil
	}
	result := map[string]summaryHeaderKey{}
	for _, name := range summaryMapKeys(headers) {
		header := headers[name]
		value, err := c.valueKey(header.Value, count, 0)
		if err != nil {
			return nil, err
		}
		result[name] = summaryHeaderKey{Value: value, Source: header.Source}
	}
	return result, nil
}

// Build a complete context key after receiver and argument expressions have already been evaluated.
func (a *analyzer) summaryKey(call CallContext, helper Function, state flow) (string, error) {
	if a.options.DisableHelperSummaries || helper.Object == nil || call.callee != nil && len(call.callee.captures) != 0 {
		return "", nil
	}
	cache := a.summaries
	count := 0
	function, err := cache.valueKey(Value{Object: helper.Object.Origin()}, &count, 0)
	if err != nil {
		return "", err
	}
	receiver, err := cache.valueKey(call.Receiver, &count, 0)
	if err != nil {
		return "", err
	}
	arguments := make([]summaryValueKey, len(call.Arguments))
	for i, arg := range call.Arguments {
		arguments[i], err = cache.valueKey(arg, &count, 0)
		if err != nil {
			return "", err
		}
	}
	var types []summaryValueKey
	if call.callee != nil {
		for _, argument := range call.callee.typeArguments {
			key, err := cache.valueKey(Value{Type: argument}, &count, 0)
			if err != nil {
				return "", err
			}
			types = append(types, key)
		}
	}
	response, err := cache.responseKey(state, &count)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(struct {
		Function, Receiver       summaryValueKey
		Arguments, TypeArguments []summaryValueKey
		Response                 summaryResponseKey
	}{function, receiver, arguments, types, response})
	if err != nil {
		return "", errMutableSummary
	}
	if len(raw) > a.options.MaxSummaryBytes {
		return "", fmt.Errorf("openapi.analysis.summary: context exceeds the summary byte budget")
	}
	return string(raw), nil
}

// Normalize response transitions independently from function identity and parameter values.
func (cache *helperSummaryCache) responseKey(state flow, count *int) (summaryResponseKey, error) {
	out := summaryResponseKey{When: state.when, Committed: state.hasCommit, Status: state.committed, Writes: state.writes, BodyKind: state.bodyKind, BodyMedia: state.bodyMedia}
	var err error
	out.Headers, err = cache.headerKey(state.headers, count)
	if err != nil {
		return out, err
	}
	out.Observed, err = cache.headerKey(state.observedHeaders, count)
	if err != nil {
		return out, err
	}
	if state.pending != nil {
		effect := *state.pending
		if effect.Codec != nil || effect.TransformSchema != nil {
			return out, errMutableSummary
		}
		key, err := cache.effectKey(effect, count)
		if err != nil {
			return out, err
		}
		out.Pending = key
	}
	return out, nil
}

// Copy finite conditions so summary storage never retains caller-owned slice backing arrays.
func copySummaryCondition(c openapi.RequestCondition) openapi.RequestCondition {
	c.Methods = append([]string(nil), c.Methods...)
	c.ExceptMethods = append([]string(nil), c.ExceptMethods...)
	c.MediaTypes = append([]string(nil), c.MediaTypes...)
	c.ExceptMediaTypes = append([]string(nil), c.ExceptMediaTypes...)
	return c
}

// Copy provenance while preserving its exact helper declaration and request domain.
func copySummarySource(source openapi.Source) openapi.Source {
	if source.When != nil {
		condition := copySummaryCondition(*source.When)
		source.When = &condition
	}
	return source
}

// Detach immutable value maps while refusing returned addresses and function capture cells.
func copySummaryValue(value Value, count *int, depth int) (Value, error) {
	if value.address != 0 || value.callable != nil {
		return Value{}, errMutableSummary
	}
	*count += 1
	if *count > 4096 || depth > 64 {
		return Value{}, fmt.Errorf("openapi.analysis.summary: results exceed the summary value budget")
	}
	if value.Fields != nil {
		fields := map[string]Value{}
		for _, name := range summaryMapKeys(value.Fields) {
			field := value.Fields[name]
			copy, err := copySummaryValue(field, count, depth+1)
			if err != nil {
				return Value{}, err
			}
			fields[name] = copy
		}
		value.Fields = fields
	}
	return value, nil
}

// Detach header values and source conditions before retaining or replaying a summary.
func copySummaryHeaders(headers map[string]HeaderValue, count *int) (map[string]HeaderValue, error) {
	if headers == nil {
		return nil, nil
	}
	result := map[string]HeaderValue{}
	for _, name := range summaryMapKeys(headers) {
		header := headers[name]
		value, err := copySummaryValue(header.Value, count, 0)
		if err != nil {
			return nil, err
		}
		result[name] = HeaderValue{Value: value, Source: copySummarySource(header.Source)}
	}
	return result, nil
}

// Detach writable effect data; registered codecs and transforms remain immutable generation rules.
func copySummaryEffect(effect Effect, count *int) (Effect, error) {
	var err error
	effect.Payload, err = copySummaryValue(effect.Payload, count, 0)
	if err != nil {
		return effect, err
	}
	effect.Headers, err = copySummaryHeaders(effect.Headers, count)
	if err != nil {
		return effect, err
	}
	effect.Source = copySummarySource(effect.Source)
	if effect.WireSchema != nil {
		effect.WireSchema, err = copyWireSchema(effect.WireSchema)
		if err != nil {
			return effect, errMutableSummary
		}
	}
	if effect.Encoding != nil {
		encoding, err := copyExplanationJSON(*effect.Encoding)
		if err != nil {
			return effect, errMutableSummary
		}
		effect.Encoding = &encoding
	}
	return effect, nil
}

// Snapshot only protocol state, excluding transient bindings, return flags, and caller diagnostics.
func copySummaryResponse(state flow, count *int) (flow, error) {
	result := flow{when: copySummaryCondition(state.when), hasCommit: state.hasCommit, writes: state.writes, bodyKind: state.bodyKind, bodyMedia: state.bodyMedia, committed: state.committed}
	var err error
	result.headers, err = copySummaryHeaders(state.headers, count)
	if err != nil {
		return result, err
	}
	result.observedHeaders, err = copySummaryHeaders(state.observedHeaders, count)
	if err != nil {
		return result, err
	}
	if state.pending != nil {
		pending, err := copySummaryEffect(*state.pending, count)
		if err != nil {
			return result, err
		}
		result.pending = &pending
	}
	return result, nil
}

// Replay correlated outcomes without restoring another invocation's local variables or capture identifiers.
func (s *helperSummary) replay(state flow) ([]evaluation, error) {
	results := make([]evaluation, 0, len(s.results))
	count := 0
	for _, stored := range s.results {
		response, err := copySummaryResponse(stored.response, &count)
		if err != nil {
			return nil, err
		}
		out := state.clone()
		out.when, out.hasCommit, out.headers, out.observedHeaders = response.when, response.hasCommit, response.headers, response.observedHeaders
		out.writes, out.bodyKind, out.bodyMedia, out.pending, out.committed = response.writes, response.bodyKind, response.bodyMedia, response.pending, response.committed
		for _, effect := range stored.effects {
			copy, err := copySummaryEffect(effect, &count)
			if err != nil {
				return nil, err
			}
			out.effects = append(out.effects, copy)
		}
		values := make([]Value, len(stored.values))
		for i, value := range stored.values {
			values[i], err = copySummaryValue(value, &count, 0)
			if err != nil {
				return nil, err
			}
		}
		results = append(results, evaluation{state: out, values: values})
	}
	return results, nil
}

// Retain successful effect transitions only when no caller cell or diagnostic was changed.
func (a *analyzer) captureSummary(base flow, results []evaluation) ([]helperSummaryResult, int, error) {
	var stored []helperSummaryResult
	count, size := 0, 0
	for _, result := range results {
		if len(result.state.diagnostics) != len(base.diagnostics) {
			return nil, 0, errMutableSummary
		}
		for id, value := range base.values {
			if !reflect.DeepEqual(value, result.state.values[id]) {
				return nil, 0, errMutableSummary
			}
		}
		response, err := copySummaryResponse(result.state, &count)
		if err != nil {
			return nil, 0, err
		}
		out := helperSummaryResult{response: response}
		for _, effect := range result.state.effects[len(base.effects):] {
			copy, err := copySummaryEffect(effect, &count)
			if err != nil {
				return nil, 0, err
			}
			out.effects = append(out.effects, copy)
		}
		for _, value := range result.values {
			copy, err := copySummaryValue(value, &count, 0)
			if err != nil {
				return nil, 0, err
			}
			out.values = append(out.values, copy)
		}
		// Charge normalized snapshots with Go identities replaced by cache-local integer keys.
		keyCount := 0
		for _, value := range out.values {
			key, err := a.summaries.valueKey(value, &keyCount, 0)
			if err != nil {
				return nil, 0, err
			}
			raw, _ := json.Marshal(key)
			if len(raw) > a.options.MaxSummaryBytes-size {
				return nil, 0, fmt.Errorf("openapi.analysis.summary: results exceed the summary byte budget")
			}
			size += len(raw)
		}
		for _, effect := range out.effects {
			key, err := a.summaries.effectKey(effect, &keyCount)
			if err != nil {
				return nil, 0, err
			}
			raw, err := json.Marshal(key)
			if err != nil {
				return nil, 0, errMutableSummary
			}
			if len(raw) > a.options.MaxSummaryBytes-size {
				return nil, 0, fmt.Errorf("openapi.analysis.summary: effects exceed the summary byte budget")
			}
			size += len(raw)
		}
		// Response storage has the same bounded value layout as its entry context.
		key, err := a.summaries.responseKey(response, &keyCount)
		if err != nil {
			return nil, 0, err
		}
		raw, err := json.Marshal(key)
		if err != nil {
			return nil, 0, errMutableSummary
		}
		if len(raw) > a.options.MaxSummaryBytes-size {
			return nil, 0, fmt.Errorf("openapi.analysis.summary: response state exceeds the summary byte budget")
		}
		size += len(raw)
		stored = append(stored, out)
	}
	return stored, size, nil
}

// Enter a new bounded context or reuse a successful summary without bypassing call and depth limits.
func (a *analyzer) summarizedFunction(call CallContext, helper Function, state flow, depth int, fallback []Value, analyze func() []evaluation) []evaluation {
	a.summaryDepth(depth + 1)
	if a.summaries == nil {
		a.summaries = newHelperSummaryCache()
	}
	key, err := a.summaryKey(call, helper, state)
	if errors.Is(err, errMutableSummary) || key == "" && err == nil {
		return analyze()
	}
	fail := func(message string) []evaluation {
		a.unknown(&state, call.Source, message)
		return []evaluation{{state: state, values: fallback}}
	}
	if err != nil {
		return fail(err.Error())
	}
	entry := a.summaries.entries[key]
	if entry != nil && entry.ready && entry.calls <= a.options.MaxCalls-a.calls && entry.depth <= a.options.MaxDepth-depth && a.ctx.Err() == nil {
		results, err := entry.replay(state)
		if err != nil {
			return fail(err.Error())
		}
		a.calls += entry.calls
		a.summaries.hits++
		a.summaryDepth(depth + entry.depth)
		return results
	}
	if entry != nil {
		return analyze()
	}
	if len(a.summaries.entries) >= a.options.MaxSummaries {
		return fail("openapi.analysis.summary: helper contexts exceed the summary count budget")
	}
	if len(key) > a.options.MaxSummaryBytes-a.summaries.bytes {
		return fail("openapi.analysis.summary: retained contexts exceed the summary byte budget")
	}
	entry = &helperSummary{}
	a.summaries.entries[key] = entry
	a.summaries.bytes += len(key)
	capture := &helperSummaryCapture{depth: depth + 1}
	a.summaryCaptures = append(a.summaryCaptures, capture)
	beforeCalls := a.calls
	results := analyze()
	a.summaryCaptures = a.summaryCaptures[:len(a.summaryCaptures)-1]
	stored, size, err := a.captureSummary(state, results)
	if errors.Is(err, errMutableSummary) {
		return results
	}
	if err == nil && size > a.options.MaxSummaryBytes-a.summaries.bytes {
		err = fmt.Errorf("openapi.analysis.summary: retained results exceed the summary byte budget")
	}
	if err != nil {
		for i := range results {
			a.unknown(&results[i].state, call.Source, err.Error())
		}
		return results
	}
	a.summaries.bytes += size
	entry.results, entry.calls, entry.depth, entry.ready = stored, a.calls-beforeCalls, capture.depth-depth, true
	return results
}
