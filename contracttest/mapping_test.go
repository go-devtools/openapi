package contracttest

import "testing"

// Mapping names may match reference keywords while remaining OpenAPI annotation data.
func TestContractMappingNamesAreNotSchemaKeywords(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","jsonSchemaDialect":"https://spec.openapis.org/oas/3.1/dialect/base","components":{"schemas":{"A":{"type":"object","discriminator":{"propertyName":"kind","mapping":{"$ref":"#/components/schemas/B","$dynamicRef":"#/components/schemas/B"}}},"B":{"type":"string"}}}}`)
	v, err := Compile(raw, "/components/schemas/A", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`{"kind":"$ref"}`)); err != nil {
		t.Fatal(err)
	}
}
