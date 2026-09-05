package compiler

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/openapi-golang/openapi/spec"
)

// 同一网络枚举值可有多个 Go 别名，说明随值聚合而不产生重复枚举。
// Aggregate descriptions for Go aliases sharing one wire value without duplicating enum values.
type enumEntry struct {
	value        any
	descriptions []string
}

// 从封闭枚举的真实常量生成值与对应说明，排序后仍保持索引一致。
// Generate values and descriptions from actual closed-enum constants while preserving alignment after sorting.
func (p *projector) annotateEnum(schema *spec.Schema, typ types.Type) error {
	entries := map[string]*enumEntry{}
	for _, pkg := range p.project.Packages {
		for _, name := range pkg.Types.Scope().Names() {
			item, ok := pkg.Types.Scope().Lookup(name).(*types.Const)
			if !ok || !types.Identical(item.Type(), typ) {
				continue
			}
			value, err := constantJSON(item)
			if err != nil {
				return err
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("openapi.enum.value: %s: %w", name, err)
			}
			key := string(encoded)
			entry := entries[key]
			if entry == nil {
				entry = &enumEntry{value: value}
				entries[key] = entry
			}
			description := strings.TrimSpace(p.project.comments[item].Summary)
			if description == "" {
				continue
			}
			duplicate := false
			for _, existing := range entry.descriptions {
				duplicate = duplicate || existing == description
			}
			if !duplicate {
				entry.descriptions = append(entry.descriptions, description)
			}
		}
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values, descriptions := make([]any, 0, len(keys)), make([]string, 0, len(keys))
	hasDescriptions := false
	for _, key := range keys {
		entry := entries[key]
		values = append(values, entry.value)
		description := strings.Join(entry.descriptions, " / ")
		descriptions = append(descriptions, description)
		hasDescriptions = hasDescriptions || description != ""
	}
	if schema.SchemaObject == nil {
		schema.SchemaObject = &spec.SchemaObject{}
	}
	schema.Enum = spec.Set(values)
	if hasDescriptions {
		if schema.Extensions == nil {
			schema.Extensions = spec.Extensions{}
		}
		encoded, err := json.Marshal(descriptions)
		if err != nil {
			return err
		}
		schema.Extensions["x-enum-descriptions"] = encoded
	}
	return nil
}

// 按 Go 常量的实际基本类型构造 JSON 值，浮点有理数不得直接写成分数字符串。
// Construct JSON values using the constants' underlying types; never serialize floating-point rationals as fractions.
func constantJSON(value *types.Const) (any, error) {
	switch value.Val().Kind() {
	case constant.String:
		return constant.StringVal(value.Val()), nil
	case constant.Int:
		return json.Number(value.Val().ExactString()), nil
	case constant.Bool:
		return constant.BoolVal(value.Val()), nil
	case constant.Float:
		v, _ := constant.Float64Val(value.Val())
		bits := 64
		if basic, ok := value.Type().Underlying().(*types.Basic); ok && basic.Kind() == types.Float32 {
			bits = 32
			v = float64(float32(v))
		}
		return json.Number(strconv.FormatFloat(v, 'g', -1, bits)), nil
	default:
		return nil, fmt.Errorf("openapi.enum.value: %s 不能编码为 JSON 枚举", value.Name())
	}
}
