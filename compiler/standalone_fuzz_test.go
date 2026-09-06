package compiler_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
)

// Validate component URI and JSON Pointer escaping and resource identity with an independent engine.
func FuzzStandaloneComponentIdentity(f *testing.F) {
	for _, name := range []string{"Name", "a/b", "a~b", "", "\u6c49\u5b57", "# %?", "~01"} {
		f.Add(name, false)
		f.Add(name, true)
	}
	f.Fuzz(func(t *testing.T, name string, identified bool) {
		if !utf8.ValidString(name) || len(name) > 128 {
			t.Skip()
		}
		pointer := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
		reference := (&url.URL{Fragment: "/components/schemas/" + pointer}).String()
		component := spec.Typed("string")
		if identified {
			component.ID = "https://example.test/value"
		}
		projection := &compiler.Projection{
			Root:       &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: reference}},
			Components: map[string]*spec.Schema{name: component},
		}
		raw, err := projection.Standalone()
		if err != nil {
			t.Fatal(err)
		}
		validator := compileStandalone(t, raw)
		if err := validator.Validate("valid"); err != nil {
			t.Fatal(err)
		}
		if validator.Validate(json.Number("42")) == nil {
			t.Fatal("export lost its string constraint")
		}
		if projection.Root.Ref != reference || component.ID != map[bool]string{false: "", true: "https://example.test/value"}[identified] {
			t.Fatal("export modified the projection")
		}
	})
}
