package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi/contracttest"
)

// Verify serialized decision tables link only contracts and diagnostics for the actual method and media type.
func TestRequestConditionLinking(t *testing.T) {
	raw := []byte(`{"formatVersion":1,"specVersion":"3.2.0","capabilities":["oas32","schema2020-12","request-conditions-v1"],"components":{},"profile":{},"templates":[{"key":"handler","operation":{"summary":"Conditional request"},"variants":[
 {"when":{"methods":["GET"]},"operation":{"responses":{"200":{"description":"query","content":{"application/json":{"schema":{"type":"string"}}}}}}},
 {"when":{"exceptMethods":["GET"],"mediaTypes":["application/json"]},"operation":{"requestBody":{"content":{"application/json":{"schema":{"type":"integer"}}}},"responses":{"201":{"description":"created","content":{"application/json":{"schema":{"type":"integer"}}}}}}},
 {"when":{"exceptMethods":["GET"],"mediaTypes":["text/plain"]},"operation":{"requestBody":{"content":{"text/plain":{"schema":{"type":"string"}}}},"responses":{"201":{"description":"created","content":{"application/json":{"schema":{"type":"string"}}}}}}},
 {"when":{"exceptMethods":["GET"],"exceptMediaTypes":["application/json","text/plain"]},"operation":{},"diagnostics":[{"code":"sample.codec.unsupported","severity":"error","message":"unsupported media"}]}
 ]}]}`)
	bundle, err := ParseBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		method    string
		media     []string
		success   bool
		status    string
		good, bad any
	}{
		{"GET", nil, true, "200", "value", 4},
		{"POST", nil, false, "", nil, nil},
		{"POST", []string{"application/json"}, true, "201", 4, "value"},
		{"POST", []string{"text/plain"}, true, "201", "value", 4},
		{"POST", []string{"application/xml"}, false, "", nil, nil},
	} {
		t.Run(sample.method+strings.Join(sample.media, ","), func(t *testing.T) {
			var route Route
			encoded, _ := json.Marshal(map[string]any{"Method": sample.method, "Path": "/value", "OperationKey": "handler", "RequestMediaTypes": sample.media})
			if err := json.Unmarshal(encoded, &route); err != nil {
				t.Fatal(err)
			}
			doc, err := Build(bundle, []Route{route}, Config{Title: "Conditional", Version: "1"})
			if !sample.success {
				if err == nil {
					t.Fatal("unresolved request condition was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			pointer := "/paths/~1value/" + strings.ToLower(sample.method) + "/responses/" + sample.status + "/content/application~1json/schema"
			validator, err := contracttest.Compile(doc.JSON(), pointer, contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err := validator.Value(sample.good); err != nil {
				t.Fatal(err)
			}
			if validator.Value(sample.bad) == nil {
				t.Fatal("unselected branch contaminated response schema")
			}
		})
	}
	var route Route
	if err := json.Unmarshal([]byte(`{"Method":"POST","Path":"/value","OperationKey":"handler","RequestMediaTypes":["application/json","text/plain"]}`), &route); err != nil {
		t.Fatal(err)
	}
	doc, err := Build(bundle, []Route{route}, Config{Title: "Multiple media", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, media := range []string{"application/json", "text/plain"} {
		if !strings.Contains(string(doc.JSON()), `"`+media+`"`) {
			t.Fatal("missing selected media")
		}
	}
	validator, err := contracttest.Compile(doc.JSON(), "/paths/~1value/post/responses/201/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if validator.Value(7) != nil || validator.Value("ready") != nil || validator.Value(false) == nil {
		t.Fatal("response alternatives were not merged accurately")
	}
	if _, err := ParseBundle([]byte(strings.Replace(string(raw), `,"request-conditions-v1"`, "", 1))); err == nil {
		t.Fatal("conditional bundle accepted without capability gate")
	}
}
