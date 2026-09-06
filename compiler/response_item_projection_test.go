package compiler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
)

// Preserve actual DTO component references across inner/outer media types while SSE data remains a protocol string.
func TestResponseItemPayloadProjection(t *testing.T) {
	for _, media := range []string{"application/x-ndjson", "text/event-stream"} {
		t.Run(media, func(t *testing.T) {
			result := compileItemFixture(t, `Emit(Item{"first"}); Emit(Other{2})`, func(c core.CallContext) ([]core.Effect, error) {
				effect := core.Effect{Kind: core.ResponseItem, Status: "200", MediaType: media, PayloadMediaType: "application/json", Payload: c.Arguments[0], Source: c.Source}
				if media == "text/event-stream" {
					effect.TransformSchema = func(payload *spec.Schema) (*spec.Schema, error) {
						data := spec.Typed("string")
						data.ContentMediaType, data.ContentSchema = "application/json", payload
						event := spec.Typed("object")
						event.Properties = map[string]*spec.Schema{"data": data, "event": spec.Typed("string")}
						event.Required = spec.Set([]string{"data"})
						return event, nil
					}
				}
				return []core.Effect{effect}, nil
			})
			doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/items", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Items", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			pointer := "/paths/~1items/get/responses/200/content/" + strings.ReplaceAll(media, "/", "~1") + "/itemSchema"
			validator, err := contracttest.Compile(doc.JSON(), pointer, contracttest.Options{AssertContent: true})
			if err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			for _, item := range []any{struct{ Name string }{"first"}, struct{ Count int }{2}} {
				if media == "text/event-stream" {
					wire.WriteString("data:")
				}
				if err := json.NewEncoder(&wire).Encode(item); err != nil {
					t.Fatal(err)
				}
				if media == "text/event-stream" {
					wire.WriteByte('\n')
				}
			}
			if media == "text/event-stream" {
				if err := validator.SSE(&wire, contracttest.Limits{}); err != nil {
					t.Fatal(err)
				}
				if validator.SSE(strings.NewReader("data:{\"Name\":7}\n\n"), contracttest.Limits{}) == nil {
					t.Fatal("JSON contentSchema was lost")
				}
				if validator.Value(map[string]any{"data": map[string]any{"Name": "wrong wire type"}}) == nil {
					t.Fatal("SSE data became an object")
				}
			} else {
				if err := validator.NDJSON(&wire, contracttest.Limits{}); err != nil {
					t.Fatal(err)
				}
				if validator.NDJSON(strings.NewReader("{\"Name\":false}\n"), contracttest.Limits{}) == nil {
					t.Fatal("projected item constraint was lost")
				}
			}
		})
	}
}

// Reusing callback input or output objects must not mutate merged items or frontend-owned schemas.
func TestResponseSchemaTransformIsolation(t *testing.T) {
	owned, shared := spec.Typed("string"), spec.Typed("string")
	calls := 0
	result := compileItemFixture(t, `Emit(Item{}); Emit(Item{})`, func(c core.CallContext) ([]core.Effect, error) {
		return []core.Effect{{Kind: core.ResponseItem, Status: "200", MediaType: "application/x-ndjson", WireSchema: owned, Source: c.Source, TransformSchema: func(input *spec.Schema) (*spec.Schema, error) {
			if input == owned || input.SchemaObject == owned.SchemaObject {
				t.Fatal("callback received caller-owned data")
			}
			input.Description = "callback input"
			calls++
			shared.Const = spec.Set[any](calls)
			shared.Type = []string{"integer"}
			return shared, nil
		}}}, nil
	})
	if owned.Description != "" {
		t.Fatal("callback mutated the frontend schema")
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/items", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Items", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	shared.Const = spec.Set[any](999)
	validator, err := contracttest.Compile(doc.JSON(), "/paths/~1items/get/responses/200/content/application~1x-ndjson/itemSchema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("callback calls: %d", calls)
	}
	if err := validator.NDJSON(strings.NewReader("1\n2\n"), contracttest.Limits{}); err != nil {
		t.Fatal(err)
	}
	if validator.Value(999) == nil {
		t.Fatal("returned callback object escaped into the contract")
	}
}

// Errors or nil results from schema transforms must prevent publication instead of becoming empty schemas.
func TestResponseSchemaTransformErrors(t *testing.T) {
	for _, fail := range []bool{false, true} {
		result := compileItemFixture(t, `Emit(Item{})`, func(c core.CallContext) ([]core.Effect, error) {
			return []core.Effect{{Kind: core.ResponseItem, Status: "200", MediaType: "application/x-ndjson", WireSchema: spec.Typed("string"), Source: c.Source, TransformSchema: func(*spec.Schema) (*spec.Schema, error) {
				if fail {
					return nil, errors.New("unresolved envelope")
				}
				return nil, nil
			}}}, nil
		})
		_, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/items", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Items", Version: "1"})
		if err == nil || !strings.Contains(err.Error(), "Schema wrapping") {
			t.Fatalf("missing transform diagnostic: %v", err)
		}
	}
}
