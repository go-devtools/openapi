package spec

import "encoding/json"

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v OpenAPI) MarshalJSON() ([]byte, error) {
	type plain OpenAPI
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *OpenAPI) UnmarshalJSON(b []byte) error {
	type plain OpenAPI
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Info) MarshalJSON() ([]byte, error) {
	type plain Info
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Info) UnmarshalJSON(b []byte) error {
	type plain Info
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Contact) MarshalJSON() ([]byte, error) {
	type plain Contact
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Contact) UnmarshalJSON(b []byte) error {
	type plain Contact
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v License) MarshalJSON() ([]byte, error) {
	type plain License
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *License) UnmarshalJSON(b []byte) error {
	type plain License
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Server) MarshalJSON() ([]byte, error) {
	type plain Server
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Server) UnmarshalJSON(b []byte) error {
	type plain Server
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v ServerVariable) MarshalJSON() ([]byte, error) {
	type plain ServerVariable
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *ServerVariable) UnmarshalJSON(b []byte) error {
	type plain ServerVariable
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Components) MarshalJSON() ([]byte, error) {
	type plain Components
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Components) UnmarshalJSON(b []byte) error {
	type plain Components
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v PathItem) MarshalJSON() ([]byte, error) {
	type plain PathItem
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *PathItem) UnmarshalJSON(b []byte) error {
	type plain PathItem
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Operation) MarshalJSON() ([]byte, error) {
	type plain Operation
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Operation) UnmarshalJSON(b []byte) error {
	type plain Operation
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v ExternalDocumentation) MarshalJSON() ([]byte, error) {
	type plain ExternalDocumentation
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *ExternalDocumentation) UnmarshalJSON(b []byte) error {
	type plain ExternalDocumentation
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Parameter) MarshalJSON() ([]byte, error) {
	type plain Parameter
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Parameter) UnmarshalJSON(b []byte) error {
	type plain Parameter
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v RequestBody) MarshalJSON() ([]byte, error) {
	type plain RequestBody
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *RequestBody) UnmarshalJSON(b []byte) error {
	type plain RequestBody
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v MediaType) MarshalJSON() ([]byte, error) {
	type plain MediaType
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *MediaType) UnmarshalJSON(b []byte) error {
	type plain MediaType
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Encoding) MarshalJSON() ([]byte, error) {
	type plain Encoding
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Encoding) UnmarshalJSON(b []byte) error {
	type plain Encoding
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Response) MarshalJSON() ([]byte, error) {
	type plain Response
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Response) UnmarshalJSON(b []byte) error {
	type plain Response
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Example) MarshalJSON() ([]byte, error) {
	type plain Example
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Example) UnmarshalJSON(b []byte) error {
	type plain Example
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Link) MarshalJSON() ([]byte, error) {
	type plain Link
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Link) UnmarshalJSON(b []byte) error {
	type plain Link
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Header) MarshalJSON() ([]byte, error) {
	type plain Header
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Header) UnmarshalJSON(b []byte) error {
	type plain Header
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Tag) MarshalJSON() ([]byte, error) {
	type plain Tag
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Tag) UnmarshalJSON(b []byte) error {
	type plain Tag
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v SecurityScheme) MarshalJSON() ([]byte, error) {
	type plain SecurityScheme
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *SecurityScheme) UnmarshalJSON(b []byte) error {
	type plain SecurityScheme
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v OAuthFlows) MarshalJSON() ([]byte, error) {
	type plain OAuthFlows
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *OAuthFlows) UnmarshalJSON(b []byte) error {
	type plain OAuthFlows
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v OAuthFlow) MarshalJSON() ([]byte, error) {
	type plain OAuthFlow
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *OAuthFlow) UnmarshalJSON(b []byte) error {
	type plain OAuthFlow
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v Discriminator) MarshalJSON() ([]byte, error) {
	type plain Discriminator
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *Discriminator) UnmarshalJSON(b []byte) error {
	type plain Discriminator
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}

// Serialize standard fields and extensions without allowing standard-field overrides.
// 序列化标准字段和扩展，禁止扩展覆盖标准语义。
func (v XML) MarshalJSON() ([]byte, error) {
	type plain XML
	return marshalExtensions(plain(v), v.Extensions)
}

// Restore standard fields and raw extension values.
// 恢复标准字段与原始扩展值。
func (v *XML) UnmarshalJSON(b []byte) error {
	type plain XML
	if err := json.Unmarshal(b, (*plain)(v)); err != nil {
		return err
	}
	ext, err := readExtensions(b)
	v.Extensions = ext
	return err
}
