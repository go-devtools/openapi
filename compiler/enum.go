package compiler

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/go-devtools/openapi/spec"
)

// Aggregate descriptions for Go aliases sharing one wire value without duplicating enum values.
type enumEntry struct {
	value        any
	descriptions []string
}

// Generate values and descriptions from actual closed-enum constants while preserving alignment after sorting.
func (p *projector) annotateEnum(schema *spec.Schema, typ types.Type) error {
	entries := map[string]*enumEntry{}
	for _, item := range p.project.constants {
		name := item.Name()
		if !types.Identical(item.Type(), typ) {
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
		doc, err := p.project.metadata(item)
		if err != nil {
			return err
		}
		description := strings.TrimSpace(doc.Summary)
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
		return nil, fmt.Errorf("openapi.enum.value: %s cannot be encoded as a JSON enum", value.Name())
	}
}
