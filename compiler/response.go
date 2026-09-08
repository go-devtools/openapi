package compiler

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"mime"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/go-devtools/openapi/spec"
)

// Copy header state so branches and committed responses do not share mutable maps.
func copyHeaders(headers map[string]HeaderValue) map[string]HeaderValue {
	copy := make(map[string]HeaderValue, len(headers))
	for name, value := range headers {
		copy[name] = value
	}
	return copy
}

// Identify bodyless statuses using protocol-level HTTP message semantics.
func bodylessStatus(status string) bool {
	code, err := strconv.Atoi(status)
	return err == nil && (code >= 100 && code < 200 || code == 204 || code == 304)
}

// Preserve current header storage for frontend observation; only pre-commit changes enter wire snapshots.
func (a *analyzer) responseHeader(state *flow, effect Effect) {
	name := textproto.CanonicalMIMEHeaderKey(effect.Name)
	if !validHeaderName(name) {
		a.unknown(state, effect.Source, "response header name is not a valid constant")
		return
	}
	if state.observedHeaders == nil {
		state.observedHeaders = copyHeaders(state.headers)
	}
	if effect.DeleteHeader {
		delete(state.observedHeaders, name)
	} else {
		if previous, exists := state.observedHeaders[name]; exists && effect.HeaderIfEmpty {
			if previous.Value.Constant == nil || previous.Value.Constant.Kind() != constant.String || constant.StringVal(previous.Value.Constant) != "" {
				return
			}
		}
		state.observedHeaders[name] = HeaderValue{Value: effect.Payload, Source: effect.Source}
	}
	if !state.hasCommit {
		state.headers = copyHeaders(state.observedHeaders)
	}
}

// Save a bodyless final response with its commit-time header snapshot once the final status is known.
func (a *analyzer) finishResponse(state *flow) {
	if state.writes != 0 || state.pending == nil {
		return
	}
	effect := *state.pending
	if !state.hasCommit {
		effect.Headers = copyHeaders(state.headers)
	}
	state.effects = append(state.effects, effect)
}

// Diagnose media-type overrides that conflict with a known rendering representation.
func (a *analyzer) checkResponseMedia(state *flow, effect Effect) {
	header, exists := state.headers["Content-Type"]
	if !exists {
		return
	}
	if header.Value.Constant == nil || header.Value.Constant.Kind() != constant.String {
		a.unknown(state, header.Source, "Dynamic Content-Type requires a centralized codec rule")
		return
	}
	actual, _, err := mime.ParseMediaType(constant.StringVal(header.Value.Constant))
	expected, _, expectedErr := mime.ParseMediaType(effect.MediaType)
	if err != nil || expectedErr != nil || actual != expected {
		a.unknown(state, header.Source, "Content-Type differs from the rendering format; register an explicit centralized codec rule")
	}
}

// Detach an explicit frontend wire schema so alternative merging cannot modify caller-owned values.
func copyWireSchema(schema *spec.Schema) (*spec.Schema, error) {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var detached spec.Schema
	if err := json.Unmarshal(raw, &detached); err != nil {
		return nil, err
	}
	return &detached, nil
}

// Merge header alternatives for one response status using values supplied by actual calls.
func mergeResponseHeaders(response *spec.Response, headers map[string]HeaderValue) error {
	for _, name := range sortedKeys(headers) {
		if strings.EqualFold(name, "Content-Type") {
			continue
		}
		value := headers[name].Value
		schema := spec.Typed("string")
		if value.Constant != nil {
			if value.Constant.Kind() != constant.String {
				return fmt.Errorf("response header %s is not a string", name)
			}
			schema.Const = spec.Set[any](constant.StringVal(value.Constant))
		}
		if response.Headers == nil {
			response.Headers = map[string]spec.RefOr[spec.Header]{}
		}
		header := response.Headers[name]
		if header.Value == nil {
			header = spec.Inline(spec.Header{})
		}
		header.Value.Schema = union(header.Value.Schema, schema)
		response.Headers[name] = header
	}
	return nil
}

// Accept only ASCII HTTP token characters in field names, excluding separators and controls.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			continue
		}
		return false
	}
	return true
}

// Represent an observed response header; Known=false still preserves header presence.
type ResponseHeaderState struct {
	Value string
	Known bool
}

// Expose pending/committed status and detached headers without exposing internal analyzer flows.
type ResponseState struct {
	// Include only headers present at the actual commit; subsequent storage mutations do not enter this snapshot.
	CommittedHeaders map[string]ResponseHeaderState
	Status           string
	Committed        bool
	Headers          map[string]ResponseHeaderState
}

// Copy only immutable strings and booleans so frontend map mutations cannot affect later analysis.
func responseSnapshot(state flow) ResponseState {
	snapshot := ResponseState{Committed: state.hasCommit, Headers: snapshotHeaderValues(state.observedHeaders)}
	if state.pending != nil {
		snapshot.Status = state.pending.Status
	}
	if state.hasCommit {
		snapshot.Status = state.committed
		snapshot.CommittedHeaders = snapshotHeaderValues(state.headers)
	}
	return snapshot
}

// Copy header names and immutable scalars so frontend snapshot mutations cannot affect analysis state.
func snapshotHeaderValues(headers map[string]HeaderValue) map[string]ResponseHeaderState {
	snapshot := map[string]ResponseHeaderState{}
	for name, header := range headers {
		value := ResponseHeaderState{}
		if header.Value.Constant != nil && header.Value.Constant.Kind() == constant.String {
			value.Known = true
			value.Value = constant.StringVal(header.Value.Constant)
		}
		snapshot[name] = value
	}
	return snapshot
}

// Project the actual payload before detaching frontend wrapping results; component references retain the shared document scope.
func (p *Project) responseSchema(effect Effect, components map[string]*spec.Schema, mappers []TypeMapper) (*spec.Schema, error) {
	var schema *spec.Schema
	var err error
	if effect.WireSchema != nil {
		schema, err = copyWireSchema(effect.WireSchema)
		p.captureProjection(p.effectUse(effect, Output), &Projection{Root: schema, Audit: []string{"The frontend supplied a wire Schema; no Go field projection was performed."}}, err)
	} else {
		media := effect.PayloadMediaType
		if media == "" {
			media = effect.MediaType
		}
		schema, err = p.valueSchema(effect.Payload, Output, media, effect.Codec, mappers, components, p.effectUse(effect, Output))
	}
	if err != nil {
		return nil, err
	}
	if effect.TransformSchema == nil {
		return schema, nil
	}
	schema, err = copyWireSchema(schema)
	if err != nil {
		return nil, err
	}
	schema, err = effect.TransformSchema(schema)
	if err != nil {
		return nil, fmt.Errorf("response Schema wrapping failed: %w", err)
	}
	if schema == nil {
		return nil, fmt.Errorf("response Schema wrapping returned nil")
	}
	return copyWireSchema(schema)
}
