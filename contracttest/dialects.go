package contracttest

import (
	"embed"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// 固定官方方言资源随可选契约测试包分发，编译阶段不访问网络。
// Fixed official dialect resources ship with the optional contract package and require no network access.
//
//go:embed dialects/*.json
var builtinDialects embed.FS

// 将固定的 OpenAPI 方言与元 Schema 加入独立引擎。
// Registers fixed OpenAPI dialects and meta-schemas with the independent engine.
func addBuiltinDialects(c *jsonschema.Compiler) error {
	entries, err := builtinDialects.ReadDir("dialects")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		raw, err := builtinDialects.ReadFile("dialects/" + entry.Name())
		if err != nil {
			return err
		}
		value, err := decode(raw)
		if err != nil {
			return err
		}
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("openapi.contract.dialect: 方言资源不是对象")
		}
		uri, ok := object["$id"].(string)
		if !ok {
			return fmt.Errorf("openapi.contract.dialect: 方言资源没有标识符")
		}
		if err = c.AddResource(uri, value); err != nil {
			return err
		}
	}
	return nil
}
