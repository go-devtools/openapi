package openapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/go-devtools/openapi/contracttest"
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

// Shared rejection variants preserve required input while explicit optional policies still conflict.
func TestConditionalRejectedBodyPresence(t *testing.T) {
	success := `{"when":{"methods":["POST"]},"operation":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object"}}}},"responses":{"201":{"description":"created"}}}}`
	for _, sample := range []struct {
		status, required string
		body, valid      bool
	}{
		{"400", "", true, true}, {"500", "", true, true}, {"400", ",\"required\":false", true, false},
		{"400", ",\"required\":true", true, true}, {"302", "", true, false}, {"default", "", true, false}, {"400", "", false, true},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/body=%t/reverse=%t", sample.status, sample.required, sample.body, reverse), func(t *testing.T) {
				body := ""
				if sample.body {
					body = `"requestBody":{"content":{"application/json":{"schema":{"type":"object"}}}` + sample.required + `},`
				}
				rejection := fmt.Sprintf(`{"when":{},"operation":{%s"responses":{"%s":{"description":"not accepted"}}}}`, body, sample.status)
				variants := success + "," + rejection
				if reverse {
					variants = rejection + "," + success
				}
				raw := fmt.Sprintf(`{"formatVersion":1,"specVersion":"3.2.0","capabilities":["oas32","schema2020-12","request-conditions-v1"],"components":{},"templates":[{"key":"handler","operation":{},"variants":[%s]}]}`, variants)
				bundle, err := ParseBundle([]byte(raw))
				if err != nil {
					t.Fatal(err)
				}
				doc, err := Build(bundle, []Route{{Method: "POST", Path: "/value", OperationKey: "handler"}}, Config{Title: "Presence", Version: "1"})
				if !sample.valid {
					if err == nil || !strings.Contains(err.Error(), "request body required differs") {
						t.Fatalf("lost explicit/accepted policy conflict: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var parsed struct {
					Paths map[string]struct {
						Post struct {
							RequestBody struct{ Required bool }
							Responses   map[string]any
						}
					}
				}
				if err := json.Unmarshal(doc.JSON(), &parsed); err != nil {
					t.Fatal(err)
				}
				op := parsed.Paths["/value"].Post
				if !op.RequestBody.Required || op.Responses[sample.status] == nil || op.Responses["201"] == nil {
					t.Fatal("rejection weakened input or lost responses")
				}
			})
		}
	}
}
