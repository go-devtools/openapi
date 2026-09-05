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

// 仅应用提交前的头设置、删除与条件设置，保留原始来源。
// Apply pre-commit header replacement, removal, and conditional insertion while preserving provenance.
func (a *analyzer) responseHeader(state *flow, effect Effect) {
	if state.hasCommit {
		return
	}
	name := textproto.CanonicalMIMEHeaderKey(effect.Name)
	if !validHeaderName(name) {
		a.unknown(state, effect.Source, "响应头名称不是有效的常量")
		return
	}
	if state.headers == nil {
		state.headers = map[string]HeaderValue{}
	}
	if effect.DeleteHeader {
		delete(state.headers, name)
		return
	}
	if previous, exists := state.headers[name]; exists && effect.HeaderIfEmpty {
		if previous.Value.Constant == nil || previous.Value.Constant.Kind() != constant.String || constant.StringVal(previous.Value.Constant) != "" {
			return
		}
	}
	state.headers[name] = HeaderValue{Value: effect.Payload, Source: effect.Source}
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
