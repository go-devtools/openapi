package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"github.com/openapi-golang/openapi/internal/comment"
	"github.com/openapi-golang/openapi/spec"
)

// 表示网络输入与输出投影方向。
type Direction string

// 输入约束与输出存在性分别计算。
const (
	Input  Direction = "request"
	Output Direction = "response"
)

// 为自定义类型提供集中映射，不执行用户编解码方法。
type TypeMapper func(ProjectionRequest) (*spec.Schema, bool, error)

// 声明一个网络字段及其实际编解码属性。
type WireField struct {
	Name          string
	Field         *types.Var
	OmitEmpty     bool
	StringEncoded bool
	Optional      bool
}

// 由适配器提供非标准字段选择规则，核心不认识框架 tag。
type WireCodec interface {
	Name() string
	Fields(*types.Struct) ([]WireField, error)
}

// 缓存身份包含类型、方向、媒体类型与 codec，非标准 codec 显式传入。
type ProjectionRequest struct {
	Type      types.Type
	Direction Direction
	MediaType string
	Codec     WireCodec
	Mappers   []TypeMapper
	MaxTypes  int
}

// 返回结构化根 Schema 与完整依赖组件。
type Projection struct {
	Root       *spec.Schema
	Components map[string]*spec.Schema
	Audit      []string
}

// 保存一次投影的私有递归缓存。
type projector struct {
	project    *Project
	request    ProjectionRequest
	components map[string]*spec.Schema
	seen       map[string]string
	count      int
}

// 从真实类型与共享注释索引生成 Schema，不执行类型的方法。
func (p *Project) Schema(request ProjectionRequest) (*Projection, error) {
	if request.Type == nil {
		return nil, fmt.Errorf("openapi.schema.type: 缺少真实 Go 类型")
	}
	if request.Direction != Input && request.Direction != Output {
		return nil, fmt.Errorf("openapi.schema.direction: 需要 request 或 response")
	}
	if request.MediaType == "" {
		request.MediaType = "application/json"
	}
	if request.Codec == nil && request.MediaType != "application/json" {
		return nil, fmt.Errorf("openapi.codec.unknown: 媒体类型 %s 需要集中 WireCodec", request.MediaType)
	}
	if request.MaxTypes == 0 {
		request.MaxTypes = 4096
	}
	pr := projector{project: p, request: request, components: map[string]*spec.Schema{}, seen: map[string]string{}}
	root, err := pr.projectType(request.Type)
	if err != nil {
		return nil, err
	}
	return &Projection{Root: root, Components: pr.components, Audit: []string{"请求 Schema 表达规范契约；未穷尽标准 JSON 对 null、大小写和固定数组的宽松接受形式。"}}, nil
}

// 导出 JSON Schema 二零二零十二，递归将组件引用转换为 $defs。
func (p *Projection) Standalone() ([]byte, error) {
	raw, err := json.Marshal(p.Root)
	if err != nil {
		return nil, err
	}
	var root any
	if err = json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	obj, ok := root.(map[string]any)
	if !ok {
		obj = map[string]any{"allOf": []any{root}}
	}
	obj["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	defs := map[string]any{}
	for name, s := range p.Components {
		b, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		var v any
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if err = dec.Decode(&v); err != nil {
			return nil, err
		}
		defs[name] = v
	}
	obj["$defs"] = defs
	var rewrite func(any)
	rewrite = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, item := range x {
				if k == "$ref" || k == "$dynamicRef" {
					if ref, ok := item.(string); ok && strings.HasPrefix(ref, "#/components/schemas/") {
						x[k] = strings.Replace(ref, "#/components/schemas/", "#/$defs/", 1)
					}
				}
				rewrite(item)
			}
		case []any:
			for _, item := range x {
				rewrite(item)
			}
		}
	}
	rewrite(obj)
	return json.Marshal(obj)
}

// 构造允许显式 null 的联合 Schema。
func nullable(s *spec.Schema) *spec.Schema {
	if s.SchemaObject != nil && len(s.Type) > 0 {
		s.Type = append(s.Type, "null")
		return s
	}
	return &spec.Schema{SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{s, spec.Typed("null")}}}
}

// 按类型身份与实际编码规则递归投影。
func (p *projector) projectType(t types.Type) (*spec.Schema, error) {
	p.count++
	if p.count > p.request.MaxTypes {
		return nil, fmt.Errorf("openapi.schema.budget: 类型图超过预算")
	}
	for _, mapper := range p.request.Mappers {
		request := p.request
		request.Type = t
		if s, ok, err := mapper(request); ok || err != nil {
			return s, err
		}
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		s, err := p.projectType(ptr.Elem())
		if err != nil {
			return nil, err
		}
		return nullable(s), nil
	}
	if named, ok := t.(*types.Named); ok {
		identity := types.TypeString(named, func(pkg *types.Package) string { return pkg.Path() })
		switch identity {
		case "time.Time":
			s := spec.Typed("string")
			s.Format = "date-time"
			return s, nil
		case "time.Duration":
			s := spec.Typed("integer")
			s.Format = "int64"
			return s, nil
		case "encoding/json.Number":
			return spec.Typed("number"), nil
		case "encoding/json.RawMessage", "encoding/json/jsontext.Value":
			return spec.Boolean(true), nil
		}
		method := "MarshalJSON"
		textMethod := "MarshalText"
		if p.request.Direction == Input {
			method = "UnmarshalJSON"
			textMethod = "UnmarshalText"
		}
		for _, mt := range []types.Type{t, types.NewPointer(t)} {
			set := types.NewMethodSet(mt)
			for i := 0; i < set.Len(); i++ {
				name := set.At(i).Obj().Name()
				if name == method || name == textMethod {
					return nil, fmt.Errorf("openapi.codec.custom: %s 定义 %s，需要方向明确的 TypeMapper", identity, name)
				}
			}
		}
		codec := "std-json"
		if p.request.Codec != nil {
			codec = p.request.Codec.Name()
		}
		key := identity + "|" + string(p.request.Direction) + "|" + p.request.MediaType + "|" + codec
		if name, ok := p.seen[key]; ok {
			return &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/" + name}}, nil
		}
		sum := sha256.Sum256([]byte(key))
		name := named.Obj().Name() + "_" + hex.EncodeToString(sum[:6])
		p.seen[key] = name
		s, err := p.projectType(named.Underlying())
		if err != nil {
			return nil, err
		}
		if err = p.annotate(s, p.project.comments[named.Obj()], false); err != nil {
			return nil, err
		}
		if doc := p.project.comments[named.Obj()]; closedEnum(doc) {
			var values []any
			for _, pkg := range p.project.Packages {
				for _, n := range pkg.Types.Scope().Names() {
					if c, ok := pkg.Types.Scope().Lookup(n).(*types.Const); ok && types.Identical(c.Type(), t) {
						switch c.Val().Kind() {
						case constant.String:
							values = append(values, constant.StringVal(c.Val()))
						case constant.Int, constant.Float:
							values = append(values, json.Number(c.Val().ExactString()))
						case constant.Bool:
							values = append(values, constant.BoolVal(c.Val()))
						}
					}
				}
			}
			sort.Slice(values, func(i, j int) bool {
				a, _ := json.Marshal(values[i])
				b, _ := json.Marshal(values[j])
				return string(a) < string(b)
			})
			if s.SchemaObject == nil {
				s.SchemaObject = &spec.SchemaObject{}
			}
			s.Enum = spec.Set(values)
		}
		p.components[name] = s
		return &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/" + name}}, nil
	}
	switch x := t.(type) {
	case *types.Basic:
		switch {
		case x.Info()&types.IsBoolean != 0:
			return spec.Typed("boolean"), nil
		case x.Info()&types.IsString != 0:
			return spec.Typed("string"), nil
		case x.Info()&types.IsInteger != 0:
			s := spec.Typed("integer")
			if x.Kind() == types.Int64 || x.Kind() == types.Uint64 {
				s.Format = "int64"
			}
			if x.Info()&types.IsUnsigned != 0 {
				s.Minimum = spec.Set(json.Number("0"))
			}
			return s, nil
		case x.Info()&types.IsFloat != 0:
			return spec.Typed("number"), nil
		case x.Kind() == types.UntypedNil:
			return spec.Typed("null"), nil
		}
	case *types.Slice:
		if basic, ok := types.Unalias(x.Elem()).(*types.Basic); ok && basic.Kind() == types.Byte {
			s := spec.Typed("string")
			s.ContentEncoding = "base64"
			return nullable(s), nil
		}
		item, err := p.projectType(x.Elem())
		if err != nil {
			return nil, err
		}
		s := spec.Typed("array")
		s.Items = item
		return nullable(s), nil
	case *types.Array:
		item, err := p.projectType(x.Elem())
		if err != nil {
			return nil, err
		}
		s := spec.Typed("array")
		s.Items = item
		s.MinItems = spec.Set(uint64(x.Len()))
		s.MaxItems = spec.Set(uint64(x.Len()))
		return s, nil
	case *types.Map:
		key := types.Unalias(x.Key())
		if named, ok := key.(*types.Named); ok {
			key = named.Underlying()
		}
		basic, ok := key.(*types.Basic)
		if !ok || basic.Info()&(types.IsString|types.IsInteger) == 0 {
			return nil, fmt.Errorf("openapi.codec.mapkey: 未知 JSON 对象键编码 %s", x.Key())
		}
		value, err := p.projectType(x.Elem())
		if err != nil {
			return nil, err
		}
		s := spec.Typed("object")
		s.AdditionalProperties = value
		if basic.Info()&types.IsInteger != 0 {
			s.PropertyNames = spec.Typed("string")
			s.PropertyNames.Pattern = "^-?[0-9]+$"
		}
		return nullable(s), nil
	case *types.Interface:
		if x.NumMethods() == 0 {
			return spec.Boolean(true), nil
		}
		return nil, fmt.Errorf("openapi.schema.interface: 非空接口需要具体实现或集中映射")
	case *types.Struct:
		var fields []WireField
		var err error
		if p.request.Codec != nil {
			fields, err = p.request.Codec.Fields(x)
		} else {
			fields, err = jsonFields(x)
		}
		if err != nil {
			return nil, err
		}
		s := spec.Typed("object")
		s.Properties = map[string]*spec.Schema{}
		required := []string{}
		for _, f := range fields {
			field, err := p.projectType(f.Field.Type())
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			if f.StringEncoded {
				base := f.Field.Type().Underlying()
				if ptr, ok := base.(*types.Pointer); ok {
					base = ptr.Elem().Underlying()
				}
				basic, ok := base.(*types.Basic)
				if !ok || basic.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) == 0 {
					return nil, fmt.Errorf("openapi.codec.string: 不支持此 ,string 类型")
				}
				field = spec.Typed("string")
				if _, ok := f.Field.Type().(*types.Pointer); ok {
					field = nullable(field)
				}
			}
			doc := p.project.comments[f.Field]
			if err = p.annotate(field, doc, true); err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			s.Properties[f.Name] = field
			if (p.request.Direction == Output && !f.OmitEmpty && !f.Optional) || flag(doc, "required") {
				required = append(required, f.Name)
			}
		}
		sort.Strings(required)
		if len(required) > 0 {
			s.Required = spec.Set(required)
		}
		return s, nil
	}
	return nil, fmt.Errorf("openapi.schema.unsupported: 无法编码类型 %s", t)
}

// 判断已解析指令中的裸或显式真标志。
func flag(doc comment.Document, key string) bool {
	for _, d := range doc.Directives {
		if string(d.Values[key]) == "true" {
			return true
		}
	}
	return false
}

// 只有类型上的裸 enum 才封闭已知常量。
func closedEnum(doc comment.Document) bool { return flag(doc, "enum") }

// 将统一约束应用到 Schema，结构事实与契约声明分别处理。
func (p *projector) annotate(s *spec.Schema, doc comment.Document, field bool) error {
	if doc.Summary == "" && len(doc.Directives) == 0 {
		return nil
	}
	if s.Bool != nil {
		if !*s.Bool {
			return fmt.Errorf("openapi.schema.annotation: 不可满足 Schema 上不能增加结构注解")
		}
		s.Bool = nil
		s.SchemaObject = &spec.SchemaObject{}
	}
	if s.SchemaObject == nil {
		s.SchemaObject = &spec.SchemaObject{}
	}
	s.Description = strings.TrimSpace(doc.Summary + "\n\n" + doc.Description)
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	for _, d := range doc.Directives {
		if d.Kind != "" {
			return fmt.Errorf("openapi.comment.context: 请求响应声明不能用于字段或类型")
		}
		for k, v := range d.Values {
			switch k {
			case "required":
				continue
			case "nonnull":
				if string(v) == "true" {
					if len(s.Type) > 0 {
						types := spec.Types{}
						for _, t := range s.Type {
							if t != "null" {
								types = append(types, t)
							}
						}
						s.Type = types
						encoded, _ := json.Marshal(types)
						obj["type"] = encoded
					}
				}
				continue
			case "nullable":
				if string(v) == "true" {
					if flag(doc, "nonnull") {
						return fmt.Errorf("openapi.comment.conflict: nullable 与 nonnull 冲突")
					}
					if len(s.Type) > 0 {
						s.Type = append(s.Type, "null")
						obj["type"], _ = json.Marshal(s.Type)
					} else {
						return fmt.Errorf("openapi.schema.nullable: 此引用联合需要集中类型扩展")
					}
				}
				continue
			case "enum":
				if string(v) == "true" {
					if field {
						return fmt.Errorf("openapi.comment.enum: 字段 enum 必须是 JSON 数组")
					}
					continue
				}
			case "ignore":
				if string(v) == "true" {
					return fmt.Errorf("openapi.schema.ignore: 字段仍会传输，不能隐藏真实字段")
				}
				continue
			case "operationId", "tags", "status", "mediaType", "type":
				return fmt.Errorf("openapi.comment.context: %s 不适用于此位置", k)
			case "writeOnly":
				if p.request.Direction == Output && string(v) == "true" {
					return fmt.Errorf("openapi.schema.writeOnly: 字段实际仍会输出")
				}
			}
			obj[k] = v
		}
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, s)
}

// 按标准 JSON 的嵌入深度、tag 优先级和冲突规则选择字段。
func jsonFields(root *types.Struct) ([]WireField, error) {
	type candidate struct {
		wire   WireField
		depth  int
		tagged bool
	}
	type level struct {
		typ       *types.Struct
		depth     int
		optional  bool
		ancestors map[*types.Struct]bool
	}
	queue := []level{{typ: root, ancestors: map[*types.Struct]bool{root: true}}}
	all := map[string][]candidate{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for i := 0; i < current.typ.NumFields(); i++ {
			f := current.typ.Field(i)
			ft := types.Unalias(f.Type())
			optional := current.optional
			if ptr, ok := ft.(*types.Pointer); ok {
				optional = true
				ft = types.Unalias(ptr.Elem())
			}
			under := ft.Underlying()
			embeddedStruct, isStruct := under.(*types.Struct)
			if !f.Exported() && (!f.Embedded() || !isStruct) {
				continue
			}
			tag := reflect.StructTag(current.typ.Tag(i)).Get("json")
			if tag == "-" {
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			tagged := name != ""
			omit, stringEncoded := false, false
			for _, option := range parts[1:] {
				omit = omit || option == "omitempty" || option == "omitzero"
				stringEncoded = stringEncoded || option == "string"
			}
			if f.Embedded() && isStruct && !tagged {
				if current.ancestors[embeddedStruct] {
					continue
				}
				ancestors := map[*types.Struct]bool{}
				for k, v := range current.ancestors {
					ancestors[k] = v
				}
				ancestors[embeddedStruct] = true
				queue = append(queue, level{typ: embeddedStruct, depth: current.depth + 1, optional: optional, ancestors: ancestors})
				continue
			}
			if name == "" {
				name = f.Name()
			}
			all[name] = append(all[name], candidate{wire: WireField{Name: name, Field: f, OmitEmpty: omit, StringEncoded: stringEncoded, Optional: current.optional}, depth: current.depth, tagged: tagged})
		}
	}
	out := []WireField{}
	for _, group := range all {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].depth != group[j].depth {
				return group[i].depth < group[j].depth
			}
			return group[i].tagged && !group[j].tagged
		})
		if len(group) > 1 && group[0].depth == group[1].depth && group[0].tagged == group[1].tagged {
			continue
		}
		out = append(out, group[0].wire)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
