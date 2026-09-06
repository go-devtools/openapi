package contracttest

import (
	"embed"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Fixed official dialect resources ship with the optional contract package and require no network access.
//
//go:embed dialects/*.json
var builtinDialects embed.FS

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
			return fmt.Errorf("openapi.contract.dialect: dialect resource is not an object")
		}
		uri, ok := object["$id"].(string)
		if !ok {
			return fmt.Errorf("openapi.contract.dialect: dialect resource has no identifier")
		}
		if err = c.AddResource(uri, value); err != nil {
			return err
		}
	}
	return nil
}
