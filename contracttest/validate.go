// 提供测试期独立契约校验；不注入业务请求中间件。
// Provides independent test-time contract validation without production middleware.
package contracttest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"

	"github.com/openapi-golang/openapi/internal/validate"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// 显式选择 format 与字符串内容断言，默认遵循二零二零十二的注解语义。
// Configures optional assertions and explicit offline resources; annotations follow draft 2020-12 by default.
type Options struct {
	AssertFormat  bool
	AssertContent bool
	MaxBytes      int64
	BaseURI       string
	Resources     map[string][]byte
	MaxResources  int
	MaxReferences int
	// 限制索引、URI 解析与诊断累计文本；零值采用十六 MiB。
	// Limits cumulative indexing, URI resolution, and diagnostic text; zero uses sixteen MiB.
	MaxIndexBytes int
	// 限制整理中间结果及最终 JSON 编码体积；零值采用十六 MiB。
	// Limits intermediate and final normalized JSON size; zero uses sixteen MiB.
	MaxNormalizedBytes int
}

// 保存独立引擎编译后的不可变 Schema；可并发验证独立样本。
// Holds an immutable compiled schema for concurrent validation of independent samples.
type Validator struct {
	schema   *jsonschema.Schema
	maxBytes int64
}

// 默认拒绝网络与文件加载，只允许显式传入的资源及引擎内置元 Schema。
// Rejects network and file loading; only explicitly supplied resources and built-in meta-schemas are available.
type offlineLoader struct{}

// 外部引用必须由调用者事先整理到本地文档，验证期间不会读取任意资源。
// External references require explicit preloading; validation never retrieves arbitrary resources.
func (offlineLoader) Load(string) (any, error) {
	return nil, fmt.Errorf("openapi.contract.external: 默认禁止加载外部引用")
}

// 在完整资源作用域中编译选中的 Schema；实例语义由独立引擎执行。
// Compile the selected schema in its complete resource scope; an independent engine evaluates instances.
func Compile(document []byte, pointer string, options Options) (*Validator, error) {
	if options.MaxBytes == 0 {
		options.MaxBytes = 8 << 20
	}
	if options.MaxBytes < 1 || int64(len(document)) > options.MaxBytes {
		return nil, fmt.Errorf("openapi.contract.budget: 文档超过字节预算")
	}
	if int64(int(options.MaxBytes)) != options.MaxBytes {
		return nil, fmt.Errorf("openapi.contract.budget: 字节预算超过平台范围")
	}
	bundle, issues := validate.BundleSchemas(document, pointer, validate.Options{BaseURI: options.BaseURI, Resources: options.Resources, MaxBytes: int(options.MaxBytes), MaxResources: options.MaxResources, MaxReferences: options.MaxReferences, MaxIndexBytes: options.MaxIndexBytes, MaxNormalizedBytes: options.MaxNormalizedBytes})
	if len(issues) > 0 {
		return nil, fmt.Errorf("openapi.contract.schema: %s %s: %s", issues[0].Code, issues[0].Path, issues[0].Message)
	}
	if err := guardJSON(bundle.Value, 256, 202048, 0); err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(offlineLoader{})
	if options.AssertFormat {
		c.AssertFormat()
	}
	if options.AssertContent {
		c.AssertContent()
	}
	if err := addBuiltinDialects(c); err != nil {
		return nil, err
	}
	resourceURIs := make([]string, 0, len(bundle.Resources))
	for uri := range bundle.Resources {
		resourceURIs = append(resourceURIs, uri)
	}
	sort.Strings(resourceURIs)
	for _, uri := range resourceURIs {
		if err := c.AddResource(uri, bundle.Resources[uri]); err != nil {
			return nil, fmt.Errorf("openapi.contract.resource: %w", err)
		}
	}
	if err := c.AddResource(bundle.URI, bundle.Value); err != nil {
		return nil, err
	}
	target, err := url.Parse(bundle.URI)
	if err != nil {
		return nil, err
	}
	target.Fragment = bundle.Pointer
	schema, err := c.Compile(target.String())
	if err != nil {
		return nil, fmt.Errorf("openapi.contract.schema: %w", err)
	}
	return &Validator{schema: schema, maxBytes: options.MaxBytes}, nil
}

// 保留 JSON 数字的十进制精度并拒绝尾随的第二个值。
// Preserves decimal precision and rejects a trailing second JSON value.
func decode(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("openapi.contract.json: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("openapi.contract.json: 预期恰好一个 JSON 值")
	}
	return value, nil
}

// 校验一段实际 JSON 网络字节，不调用业务类型的序列化方法。
// Validates actual JSON wire bytes without invoking application serialization methods.
func (v *Validator) JSON(raw []byte) error {
	if v == nil || v.schema == nil {
		return fmt.Errorf("openapi.contract.nil: 未初始化验证器")
	}
	if int64(len(raw)) > v.maxBytes {
		return fmt.Errorf("openapi.contract.budget: 样本超过字节预算")
	}
	value, err := decode(raw)
	if err != nil {
		return err
	}
	return v.Value(value)
}

// 校验已经解析为 JSON 数据模型的样本。
// Validates a sample already represented by the JSON data model.
func (v *Validator) Value(value any) error {
	if v == nil || v.schema == nil {
		return fmt.Errorf("openapi.contract.nil: 未初始化验证器")
	}
	if err := guardJSON(value, 128, 200000, v.maxBytes); err != nil {
		return err
	}
	if err := v.schema.Validate(value); err != nil {
		return fmt.Errorf("openapi.contract.instance: %w", err)
	}
	return nil
}
