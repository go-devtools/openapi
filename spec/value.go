// Provide native OAS 3.2 models with lossless JSON presence semantics.
// 提供原生 OpenAPI 三点二模型及无损 JSON 存在性表示。
package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Use the standard default dialect rather than deriving it from the OpenAPI version.
// 标准规定的默认 Schema 方言，不由 OpenAPI 版本字符串拼接。
const DefaultDialect = "https://spec.openapis.org/oas/3.1/dialect/base"

// Distinguish absence from explicit zero, empty collections, and null.
// 区分字段不存在与字段显式为零、空集合或 null。
type Optional[T any] struct {
	Value   T
	Present bool
}

// Construct an explicitly present value.
// 构造一个明确存在的字段。
func Set[T any](value T) Optional[T] { return Optional[T]{Value: value, Present: true} }

// Support encoding/json omitzero presence checks.
// 供 encoding/json 的 omitzero 判断字段是否省略。
func (v Optional[T]) IsZero() bool { return !v.Present }

// Serialize a present value without dropping JSON zero values.
// 序列化已存在的值，保留其 JSON 零值。
func (v Optional[T]) MarshalJSON() ([]byte, error) { return json.Marshal(v.Value) }

// Restore explicit presence from JSON.
// 从 JSON 恢复显式存在标志。
func (v *Optional[T]) UnmarshalJSON(b []byte) error {
	v.Present = true
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(&v.Value)
}

// Represent a single Schema type or a type union.
// 表示 Schema 类型关键字的单值或联合形式。
type Types []string

// Encode one type as a string and multiple types as an array.
// 单类型输出字符串，多类型输出数组。
func (t Types) MarshalJSON() ([]byte, error) {
	if len(t) == 1 {
		return json.Marshal(t[0])
	}
	return json.Marshal([]string(t))
}

// Accept standard single-type and type-array forms.
// 接受标准允许的单类型和类型数组。
func (t *Types) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = Types{s}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(t))
}

// Store x- extensions as valid raw JSON.
// 保存标准对象的 x- 扩展，值仍为合法 JSON。
type Extensions map[string]json.RawMessage

// Represent a Reference Object separately from Schema references.
// 表示标准 Reference Object，与 Schema 中的引用独立。
type Reference struct {
	Ref         string `json:"$ref"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
}

// Distinguish a reference from an inline standard object.
// 明确区分引用对象与内联标准对象。
type RefOr[T any] struct {
	Reference *Reference
	Value     *T
}

// Construct an inline standard object.
// 构造内联标准对象。
func Inline[T any](v T) RefOr[T] { return RefOr[T]{Value: &v} }

// Construct a standard Reference Object.
// 构造标准引用对象。
func Ref[T any](ref string) RefOr[T] { return RefOr[T]{Reference: &Reference{Ref: ref}} }

// Reject empty or conflicting branches during serialization.
// 防止双分支或空分支被静默序列化。
func (r RefOr[T]) MarshalJSON() ([]byte, error) {
	if (r.Reference == nil) == (r.Value == nil) {
		return nil, fmt.Errorf("reference must select exactly one branch")
	}
	if r.Reference != nil {
		return json.Marshal(r.Reference)
	}
	return json.Marshal(r.Value)
}

// Recognize reference objects by $ref without treating siblings as inline objects.
// 按 $ref 区分引用，不把引用兄弟字段当内联对象。
func (r *RefOr[T]) UnmarshalJSON(b []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	if keys == nil {
		return fmt.Errorf("standard object must not be null")
	}
	*r = RefOr[T]{}
	if _, ok := keys["$ref"]; ok {
		r.Reference = &Reference{}
		return json.Unmarshal(b, r.Reference)
	}
	r.Value = new(T)
	return json.Unmarshal(b, r.Value)
}

// Merge extensions while refusing to replace standard fields.
// 合并对象扩展，并拒绝覆盖标准字段。
func marshalExtensions(v any, ext Extensions) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(ext) == 0 {
		return b, nil
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	for k, v := range ext {
		if len(k) < 2 || k[:2] != "x-" {
			return nil, fmt.Errorf("extension must start with x-: %s", k)
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("extension value is not JSON: %s", k)
		}
		obj[k] = v
	}
	return json.Marshal(obj)
}

// Decode extension fields separately from the standard typed fields.
// 只恢复扩展字段，标准字段由类型自身解码。
func readExtensions(b []byte) (Extensions, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var ext Extensions
	for k, v := range raw {
		if len(k) >= 2 && k[:2] == "x-" {
			if ext == nil {
				ext = Extensions{}
			}
			ext[k] = bytes.Clone(v)
		}
	}
	return ext, nil
}
