package spec

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Test lossless serialization of native OAS 3.2 fields and explicit zero values.
// 验证原生三点二字段与显式零值能够无损序列化。
func TestNative32AndPresence(t *testing.T) {
	fixture, err := os.ReadFile("../testdata/golden/openapi32-full.json")
	if err != nil {
		t.Fatal(err)
	}
	var native OpenAPI
	if err = json.Unmarshal(fixture, &native); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	var before, after any
	for _, entry := range []struct {
		raw   []byte
		value *any
	}{{fixture, &before}, {roundtrip, &after}} {
		decoder := json.NewDecoder(bytes.NewReader(entry.raw))
		decoder.UseNumber()
		if err = decoder.Decode(entry.value); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("full OpenAPI 3.2 fixture lost fields or presence during roundtrip")
	}
	doc := OpenAPI{OpenAPI: "3.2.0", Self: "https://example.test/api", JSONSchemaDialect: DefaultDialect,
		Info: Info{Title: "User endpoints", Version: "1"}, Security: Set([]SecurityRequirement{}),
		Paths:      map[string]*PathItem{"/users": {Query: &Operation{Responses: map[string]RefOr[Response]{"200": Inline(Response{Description: "Success", Content: map[string]RefOr[MediaType]{"application/x-ndjson": Inline(MediaType{ItemSchema: Boolean(false)})}})}}}},
		Components: &Components{Schemas: map[string]*Schema{"Empty": {SchemaObject: &SchemaObject{Type: Types{"integer"}, Minimum: Set(json.Number("0")), Const: Set[any](nil), Examples: Set([]any{})}}}},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"query":`, `"itemSchema":false`, `"minimum":0`, `"const":null`, `"examples":[]`, `"security":[]`, `"$self":`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s: %s", want, b)
		}
	}
	var decoded OpenAPI
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(decoded)
	if err != nil || string(again) != string(b) {
		t.Fatalf("roundtrip lost data: %s, %v", again, err)
	}
}

// Keep boolean Schemas, type unions, and Reference Objects distinct.
// 验证 Schema 布尔分支、类型联合与引用对象不会混淆。
func TestSchemaAndReferenceRoundTrip(t *testing.T) {
	for _, src := range []string{`false`, `true`, `{}`, `{"type":["string","null"]}`, `{"$ref":"#/$defs/X","minLength":0}`} {
		var s Schema
		if err := json.Unmarshal([]byte(src), &s); err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var x, y any
		_ = json.Unmarshal([]byte(src), &x)
		_ = json.Unmarshal(b, &y)
		xb, _ := json.Marshal(x)
		yb, _ := json.Marshal(y)
		if string(xb) != string(yb) {
			t.Errorf("roundtrip differs: %s / %s", xb, yb)
		}
	}
	r := Ref[Response]("#/components/responses/Bad")
	b, err := json.Marshal(r)
	if err != nil || string(b) != `{"$ref":"#/components/responses/Bad"}` {
		t.Fatalf("reference error: %s %v", b, err)
	}
	var s Schema
	if json.Unmarshal([]byte(`42`), &s) == nil {
		t.Fatal("numeric Schema was incorrectly accepted")
	}
}
