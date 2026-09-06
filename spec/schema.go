package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Represent exactly one boolean or object Schema branch.
// 表示布尔 Schema 或结构化 Schema，两者不能同时设置。
type Schema struct {
	Bool *bool
	*SchemaObject
}

// Build a boolean Schema: false rejects every instance and true imposes no constraints.
// 构造布尔 Schema；false 拒绝任何实例，true 不施加约束。
func Boolean(v bool) *Schema { return &Schema{Bool: &v} }

// Build a Schema with the specified types.
// 构造具有给定类型的 Schema。
func Typed(types ...string) *Schema { return &Schema{SchemaObject: &SchemaObject{Type: Types(types)}} }

// Preserve boolean branches and keyword presence during serialization.
// 保留布尔分支及所有 JSON Schema 关键字的存在性。
func (s Schema) MarshalJSON() ([]byte, error) {
	if s.Bool != nil {
		if s.SchemaObject != nil {
			return nil, fmt.Errorf("Schema branch conflict")
		}
		return json.Marshal(*s.Bool)
	}
	if s.SchemaObject == nil {
		return []byte("{}"), nil
	}
	type plain SchemaObject
	return marshalExtensions((*plain)(s.SchemaObject), s.Extensions)
}

// Reject null, arrays, and numbers as Schema roots.
// 拒绝 null、数组和数字等非法 Schema 根值。
func (s *Schema) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*s = Schema{}
	if bytes.Equal(b, []byte("true")) || bytes.Equal(b, []byte("false")) {
		var v bool
		_ = json.Unmarshal(b, &v)
		s.Bool = &v
		return nil
	}
	if len(b) == 0 || b[0] != '{' {
		return fmt.Errorf("Schema must be an object or boolean")
	}
	s.SchemaObject = &SchemaObject{}
	if err := json.Unmarshal(b, s.SchemaObject); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	s.Extensions = ext
	return err
}

// Expose JSON Schema 2020-12 and OpenAPI Schema fields.
// 覆盖 JSON Schema 二零二零十二及 OpenAPI 的结构化 Schema 字段。
type SchemaObject struct {
	Schema                string                 `json:"$schema,omitempty"`
	ID                    string                 `json:"$id,omitempty"`
	Ref                   string                 `json:"$ref,omitempty"`
	Anchor                string                 `json:"$anchor,omitempty"`
	DynamicAnchor         string                 `json:"$dynamicAnchor,omitempty"`
	DynamicRef            string                 `json:"$dynamicRef,omitempty"`
	Vocabulary            map[string]bool        `json:"$vocabulary,omitempty"`
	Defs                  map[string]*Schema     `json:"$defs,omitempty"`
	Comment               string                 `json:"$comment,omitempty"`
	Type                  Types                  `json:"type,omitempty"`
	Title                 string                 `json:"title,omitempty"`
	Description           string                 `json:"description,omitempty"`
	Enum                  Optional[[]any]        `json:"enum,omitzero"`
	Const                 Optional[any]          `json:"const,omitzero"`
	Default               Optional[any]          `json:"default,omitzero"`
	Examples              Optional[[]any]        `json:"examples,omitzero"`
	Format                string                 `json:"format,omitempty"`
	Deprecated            bool                   `json:"deprecated,omitempty"`
	ReadOnly              bool                   `json:"readOnly,omitempty"`
	WriteOnly             bool                   `json:"writeOnly,omitempty"`
	MultipleOf            Optional[json.Number]  `json:"multipleOf,omitzero"`
	Minimum               Optional[json.Number]  `json:"minimum,omitzero"`
	Maximum               Optional[json.Number]  `json:"maximum,omitzero"`
	ExclusiveMinimum      Optional[json.Number]  `json:"exclusiveMinimum,omitzero"`
	ExclusiveMaximum      Optional[json.Number]  `json:"exclusiveMaximum,omitzero"`
	MinLength             Optional[uint64]       `json:"minLength,omitzero"`
	MaxLength             Optional[uint64]       `json:"maxLength,omitzero"`
	Pattern               string                 `json:"pattern,omitempty"`
	ContentEncoding       string                 `json:"contentEncoding,omitempty"`
	ContentMediaType      string                 `json:"contentMediaType,omitempty"`
	ContentSchema         *Schema                `json:"contentSchema,omitempty"`
	Items                 *Schema                `json:"items,omitempty"`
	PrefixItems           []*Schema              `json:"prefixItems,omitempty"`
	Contains              *Schema                `json:"contains,omitempty"`
	MinContains           Optional[uint64]       `json:"minContains,omitzero"`
	MaxContains           Optional[uint64]       `json:"maxContains,omitzero"`
	MinItems              Optional[uint64]       `json:"minItems,omitzero"`
	MaxItems              Optional[uint64]       `json:"maxItems,omitzero"`
	UniqueItems           bool                   `json:"uniqueItems,omitempty"`
	UnevaluatedItems      *Schema                `json:"unevaluatedItems,omitempty"`
	Properties            map[string]*Schema     `json:"properties,omitempty"`
	PatternProperties     map[string]*Schema     `json:"patternProperties,omitempty"`
	AdditionalProperties  *Schema                `json:"additionalProperties,omitempty"`
	UnevaluatedProperties *Schema                `json:"unevaluatedProperties,omitempty"`
	PropertyNames         *Schema                `json:"propertyNames,omitempty"`
	Required              Optional[[]string]     `json:"required,omitzero"`
	MinProperties         Optional[uint64]       `json:"minProperties,omitzero"`
	MaxProperties         Optional[uint64]       `json:"maxProperties,omitzero"`
	DependentRequired     map[string][]string    `json:"dependentRequired,omitempty"`
	DependentSchemas      map[string]*Schema     `json:"dependentSchemas,omitempty"`
	AllOf                 []*Schema              `json:"allOf,omitempty"`
	AnyOf                 []*Schema              `json:"anyOf,omitempty"`
	OneOf                 []*Schema              `json:"oneOf,omitempty"`
	Not                   *Schema                `json:"not,omitempty"`
	If                    *Schema                `json:"if,omitempty"`
	Then                  *Schema                `json:"then,omitempty"`
	Else                  *Schema                `json:"else,omitempty"`
	Discriminator         *Discriminator         `json:"discriminator,omitempty"`
	XML                   *XML                   `json:"xml,omitempty"`
	ExternalDocs          *ExternalDocumentation `json:"externalDocs,omitempty"`
	Extensions            Extensions             `json:"-"`
}

// Describe polymorphic dispatch without upgrading anyOf to oneOf.
// 表示多态分派，不将 anyOf 自动升级为 oneOf。
type Discriminator struct {
	PropertyName   string            `json:"propertyName,omitempty"`
	Mapping        map[string]string `json:"mapping,omitempty"`
	DefaultMapping string            `json:"defaultMapping,omitempty"`
	Extensions     Extensions        `json:"-"`
}

// Describe OAS 3.2 XML node kinds and namespaces.
// 表示三点二 XML 节点类型及名称空间。
type XML struct {
	Name       string     `json:"name,omitempty"`
	Namespace  string     `json:"namespace,omitempty"`
	Prefix     string     `json:"prefix,omitempty"`
	Attribute  bool       `json:"attribute,omitempty"`
	Wrapped    bool       `json:"wrapped,omitempty"`
	NodeType   string     `json:"nodeType,omitempty"`
	Extensions Extensions `json:"-"`
}
