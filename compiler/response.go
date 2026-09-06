package compiler

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"mime"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/openapi-golang/openapi/spec"
)

// 复制响应头状态，分支和已提交的响应不能共享可变映射。
// Copy header state so branches and committed responses do not share mutable maps.
func copyHeaders(headers map[string]HeaderValue) map[string]HeaderValue {
	copy := make(map[string]HeaderValue, len(headers))
	for name, value := range headers {
		copy[name] = value
	}
	return copy
}

// 按 HTTP 状态的通用消息语义识别没有响应体的状态。
// Identify bodyless statuses using protocol-level HTTP message semantics.
func bodylessStatus(status string) bool {
	code, err := strconv.Atoi(status)
	return err == nil && (code >= 100 && code < 200 || code == 204 || code == 304)
}

// 保留当前头存储供前端观察，只有提交前的修改进入网络头快照。
// Preserve current header storage for frontend observation; only pre-commit changes enter wire snapshots.
func (a *analyzer) responseHeader(state *flow, effect Effect) {
	name := textproto.CanonicalMIMEHeaderKey(effect.Name)
	if !validHeaderName(name) {
		a.unknown(state, effect.Source, "响应头名称不是有效的常量")
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

// 在最终状态确定后保存未写 body 的响应及其提交时头快照。
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

// 检测与已知渲染格式不一致的媒体类型覆盖，不能假装编码事实不变。
// Diagnose media-type overrides that conflict with a known rendering representation.
func (a *analyzer) checkResponseMedia(state *flow, effect Effect) {
	header, exists := state.headers["Content-Type"]
	if !exists {
		return
	}
	if header.Value.Constant == nil || header.Value.Constant.Kind() != constant.String {
		a.unknown(state, header.Source, "动态 Content-Type 需要集中编解码规则")
		return
	}
	actual, _, err := mime.ParseMediaType(constant.StringVal(header.Value.Constant))
	expected, _, expectedErr := mime.ParseMediaType(effect.MediaType)
	if err != nil || expectedErr != nil || actual != expected {
		a.unknown(state, header.Source, "Content-Type 覆盖与渲染格式不同，需要明确的集中编解码规则")
	}
}

// 脱离前端拥有的明确网络 Schema，避免备选合并修改调用方对象。
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

// 将同一响应状态的头备选合并，常量值仍由实际调用提供。
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
				return fmt.Errorf("响应头 %s 不是字符串", name)
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

// HTTP 字段名称仅接受 ASCII token 字符，拒绝分隔符和控制字符。
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

// 表示已观察到的响应头；Known 为 false 时仍保留字段存在事实。
// Represent an observed response header; Known=false still preserves header presence.
type ResponseHeaderState struct {
	Value string
	Known bool
}

// 提供待提交或已提交状态及独立的响应头快照，不暴露分析器内部流。
// Expose pending/committed status and detached headers without exposing internal analyzer flows.
type ResponseState struct {
	Status    string
	Committed bool
	Headers   map[string]ResponseHeaderState
}

// 只复制不可变字符串与布尔值，前端修改映射不能影响后续分析。
// Copy only immutable strings and booleans so frontend map mutations cannot affect later analysis.
func responseSnapshot(state flow) ResponseState {
	snapshot := ResponseState{Committed: state.hasCommit, Headers: map[string]ResponseHeaderState{}}
	if state.pending != nil {
		snapshot.Status = state.pending.Status
	}
	if state.hasCommit {
		snapshot.Status = state.committed
	}
	for name, header := range state.observedHeaders {
		value := ResponseHeaderState{}
		if header.Value.Constant != nil && header.Value.Constant.Kind() == constant.String {
			value.Known = true
			value.Value = constant.StringVal(header.Value.Constant)
		}
		snapshot.Headers[name] = value
	}
	return snapshot
}

// 先投影实际载荷，再隔离前端包装结果，组件引用继续使用共同文档作用域。
// Project the actual payload before detaching frontend wrapping results; component references retain the shared document scope.
func (p *Project) responseSchema(effect Effect, components map[string]*spec.Schema, mappers []TypeMapper) (*spec.Schema, error) {
	var schema *spec.Schema
	var err error
	if effect.WireSchema != nil {
		schema, err = copyWireSchema(effect.WireSchema)
	} else {
		media := effect.PayloadMediaType
		if media == "" {
			media = effect.MediaType
		}
		schema, err = p.valueSchema(effect.Payload, Output, media, effect.Codec, mappers, components)
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
		return nil, fmt.Errorf("响应 Schema 包装失败: %w", err)
	}
	if schema == nil {
		return nil, fmt.Errorf("响应 Schema 包装返回 nil")
	}
	return copyWireSchema(schema)
}
