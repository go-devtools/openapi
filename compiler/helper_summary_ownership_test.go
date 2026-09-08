package compiler

import (
	"go/constant"
	"go/types"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// Consumer edits to one replay cannot alter stored values, protocol state, or the next replay.
func TestHelperSummaryReplayOwnership(t *testing.T) {
	payload := Value{Type: types.Typ[types.String], Fields: map[string]Value{"Name": {Type: types.Typ[types.String], Constant: constant.MakeString("Ada")}}}
	source := openapi.Source{File: "handler.go", Line: 7, When: &openapi.RequestCondition{Methods: []string{"GET"}}}
	schema := spec.Typed("object")
	schema.Properties = map[string]*spec.Schema{"Name": spec.Typed("string")}
	summary := helperSummary{results: []helperSummaryResult{{response: flow{when: openapi.RequestCondition{Methods: []string{"GET"}}, hasCommit: true, committed: "201", headers: map[string]HeaderValue{"X-Name": {Value: payload, Source: source}}, pending: &Effect{Kind: ResponseStatus, Status: "201", Source: source}}, effects: []Effect{{Kind: ResponseBody, Status: "201", Payload: payload, WireSchema: schema, Source: source}}, values: []Value{payload}}}}
	base := flow{values: map[uint64]Value{1: payload}, bindings: map[types.Object]uint64{}, effects: []Effect{{Kind: RequestBody, Source: source}}}
	first, err := summary.replay(base)
	if err != nil {
		t.Fatal(err)
	}
	first[0].values[0].Fields["Name"] = Value{Constant: constant.MakeString("changed")}
	first[0].state.effects[1].WireSchema.Properties["Name"].Type = []string{"integer"}
	first[0].state.effects[1].Payload.Fields["Name"] = Value{Constant: constant.MakeString("changed")}
	first[0].state.effects[1].Source.When.Methods[0] = "POST"
	first[0].state.when.Methods[0] = "DELETE"
	header := first[0].state.headers["X-Name"]
	header.Source.When.Methods[0] = "PUT"
	header.Value.Fields["Name"] = Value{Constant: constant.MakeString("changed")}
	first[0].state.pending.Source.When.Methods[0] = "PATCH"
	second, err := summary.replay(base)
	if err != nil {
		t.Fatal(err)
	}
	if constant.StringVal(second[0].values[0].Fields["Name"].Constant) != "Ada" || constant.StringVal(second[0].state.effects[1].Payload.Fields["Name"].Constant) != "Ada" {
		t.Fatal("replay value maps alias cached storage")
	}
	if second[0].state.effects[1].WireSchema.Properties["Name"].Type[0] != "string" {
		t.Fatal("replay schema aliases cached storage")
	}
	for _, actual := range []string{second[0].state.when.Methods[0], second[0].state.effects[1].Source.When.Methods[0], second[0].state.headers["X-Name"].Source.When.Methods[0], second[0].state.pending.Source.When.Methods[0]} {
		if actual != "GET" {
			t.Fatalf("replay source condition was modified: %s", actual)
		}
	}
	if constant.StringVal(base.values[1].Fields["Name"].Constant) != "Ada" || len(base.effects) != 1 {
		t.Fatal("replay modified caller state")
	}
}

// Wide abstract facts fail explicitly instead of exhausting traversal or retaining partial values.
func TestHelperSummaryFactBudget(t *testing.T) {
	wide := Value{Fields: map[string]Value{}}
	for i := 0; i < 4100; i++ {
		wide.Fields[constant.MakeInt64(int64(i)).ExactString()] = Value{Type: types.Typ[types.Int]}
	}
	cache := newHelperSummaryCache()
	count := 0
	if _, err := cache.valueKey(wide, &count, 0); err == nil || err == errMutableSummary {
		t.Fatalf("wide input was not a budget error: %v", err)
	}
	count = 0
	if _, err := copySummaryValue(wide, &count, 0); err == nil || err == errMutableSummary {
		t.Fatalf("wide result was not a budget error: %v", err)
	}
	deep := Value{Type: types.Typ[types.Int]}
	for i := 0; i < 66; i++ {
		deep = Value{Fields: map[string]Value{"Next": deep}}
	}
	count = 0
	if _, err := cache.valueKey(deep, &count, 0); err == nil || err == errMutableSummary {
		t.Fatalf("deep input was not a budget error: %v", err)
	}
	count = 0
	if _, err := copySummaryValue(Value{address: 7}, &count, 0); err != errMutableSummary {
		t.Fatalf("tracked address must fall back to source analysis: %v", err)
	}
}
