package compiler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/testdata/helpergeneric"
)

// Register a neutral emitter by complete package and method identity.
func genericFrontend() core.Frontend {
	return core.Frontend{Name: "neutral-generic-helper-v1", Match: func(f core.Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, Call: func(c core.CallContext) ([]core.Effect, error) {
		if c.Object == nil || c.Object.Pkg() == nil || c.Object.Pkg().Path() != "github.com/openapi-golang/openapi/testdata/helpergeneric" || c.Object.FullName() != "(*github.com/openapi-golang/openapi/testdata/helpergeneric.Channel).Emit" {
			return nil, nil
		}
		status := ""
		if c.Arguments[0].Constant != nil {
			status = c.Arguments[0].Constant.ExactString()
		}
		return []core.Effect{{Kind: core.ResponseBody, Status: status, MediaType: "application/json", Payload: c.Arguments[1], Source: c.Source}}, nil
	}, Return: func(c core.ReturnContext) ([]core.Effect, error) {
		if len(c.Values) == 0 {
			return nil, nil
		}
		return []core.Effect{{Kind: core.ResponseBody, Status: "200", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
	}}
}

// Build the selected candidate and independently validate the generated response schema.
func genericContract(t *testing.T, result *core.Result, name, status string) *contracttest.Validator {
	t.Helper()
	for _, entry := range result.Bundle.Index() {
		if !strings.HasSuffix(entry.Symbol, "."+name) {
			continue
		}
		doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Generic helpers", Version: "1"})
		if err != nil {
			t.Fatal(err)
		}
		validator, err := contracttest.Compile(doc.JSON(), "/paths/~1value/get/responses/"+status+"/content/application~1json/schema", contracttest.Options{})
		if err != nil {
			t.Fatal(err)
		}
		return validator
	}
	t.Fatalf("missing actual candidate %s", name)
	return nil
}

// Compare real helper results with concrete schemas and reject mismatched types and lost field contracts.
func TestGenericHelperSubstitution(t *testing.T) {
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: "../testdata/helpergeneric", Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{genericFrontend()}, Explain: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name string
		call func() any
		bad  []string
	}{
		{"HZero", helpergeneric.HZero, []string{`"0"`, `null`}},
		{"HNamedZero", helpergeneric.HNamedZero, []string{`"0"`, `null`}},
		{"HExplicit", helpergeneric.HExplicit, []string{`{"Data":{"Name":7}}`, `{"Data":{"Name":"x"}}`}},
		{"HInferred", helpergeneric.HInferred, []string{`{"Data":7}`}},
		{"HFunctionValue", helpergeneric.HFunctionValue, []string{`"7"`}},
		{"HEnvelopeValue", helpergeneric.HEnvelopeValue, []string{`{"Data":"Ada"}`}},
		{"HMultiple", helpergeneric.HMultiple, []string{`{"First":{"Name":"Ada"},"Second":"7"}`}},
		{"HClosure", helpergeneric.HClosure, []string{`{"Data":true}`}},
		{"HNested", helpergeneric.HNested, []string{`{"Data":{"Data":{"Name":"x"}}}`}},
		{"HAnonymous", helpergeneric.HAnonymous, []string{`{"Value":{"Name":"Ada"},"Label":"x"}`, `{"Value":7,"Label":"Ada"}`}},
		{"HConverted", helpergeneric.HConverted, []string{`true`}},
		{"HMethod", helpergeneric.HMethod, []string{`{"Name":7}`}},
		{"HMethodExpression", helpergeneric.HMethodExpression, []string{`{"Name":7}`}},
		{"HRecursive", helpergeneric.HRecursive, []string{`{"Data":false}`}},
		{"HFieldContract", helpergeneric.HFieldContract, []string{`{"Value":"x"}`, `{"Value":7}`}},
		{"HNilPointer", helpergeneric.HNilPointer, []string{`{}`}},
		{"HNilSlice", helpergeneric.HNilSlice, []string{`[]`}},
		{"HNilMap", helpergeneric.HNilMap, []string{`{}`}},
		{"HEmptyArray", helpergeneric.HEmptyArray, []string{`null`, `[{"Name":"Ada"}]`}},
		{"HAlias", helpergeneric.HAlias, []string{`{"Data":7}`}},
		{"HCollections", helpergeneric.HCollections, []string{`{"Pointer":{"Name":"Ada"},"Array":[{"Name":"Ada"}],"Slice":[{"Name":7}],"Map":{"first":{"Name":"Ada"}}}`}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			validator := genericContract(t, result, sample.name, "200")
			raw, err := json.Marshal(sample.call())
			if err != nil {
				t.Fatal(err)
			}
			if err = validator.JSON(raw); err != nil {
				t.Fatal(err)
			}
			for _, bad := range sample.bad {
				if validator.JSON([]byte(bad)) == nil {
					t.Errorf("accepted invalid payload %s", bad)
				}
			}
		})
	}
}

// Keep helper status constants, method side effects, and separate generic instances correlated.
func TestGenericHelperResponseEffects(t *testing.T) {
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: "../testdata/helpergeneric", Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{genericFrontend()}})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name, status, bad string
		call              func(*helpergeneric.Channel)
	}{
		{"HSend", "201", `{"Data":"wrong"}`, helpergeneric.HSend},
		{"HSendOther", "202", `{"Data":7}`, helpergeneric.HSendOther},
		{"HMethodStatus", "207", `{"Name":7}`, helpergeneric.HMethodStatus},
		{"HPointerMethodStatus", "206", `{"Name":7}`, helpergeneric.HPointerMethodStatus},
	} {
		t.Run(sample.name, func(t *testing.T) {
			channel := new(helpergeneric.Channel)
			sample.call(channel)
			code, _ := json.Marshal(channel.Code)
			if string(code) != sample.status {
				t.Fatalf("actual status=%s", code)
			}
			validator := genericContract(t, result, sample.name, sample.status)
			raw, err := json.Marshal(channel.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err = validator.JSON(raw); err != nil {
				t.Fatal(err)
			}
			if validator.JSON([]byte(sample.bad)) == nil {
				t.Fatal("generic effect lost its concrete payload type")
			}
		})
	}
}

// Branch-specific instantiations retain their own payload schemas and declaration evidence.
func TestGenericHelperBranchesAndOrigins(t *testing.T) {
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: "../testdata/helpergeneric", Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{genericFrontend()}, Explain: true})
	if err != nil {
		t.Fatal(err)
	}
	validator := genericContract(t, result, "HBranches", "200")
	for _, flag := range []bool{false, true} {
		raw, err := json.Marshal(helpergeneric.HBranches(flag))
		if err != nil {
			t.Fatal(err)
		}
		if err = validator.JSON(raw); err != nil {
			t.Fatal(err)
		}
	}
	if validator.JSON([]byte(`{"Data":7}`)) == nil {
		t.Fatal("separate helper instantiations broadened their branch schemas")
	}
	explanation, err := result.Explain(core.ExplainQuery{Symbol: "github.com/openapi-golang/openapi/testdata/helpergeneric.fieldContract.Value"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, use := range explanation.Uses {
		for _, origin := range use.Origins {
			if origin.Source.Symbol == explanation.Symbol && origin.GoType == "string" && len(origin.Declarations) > 0 && origin.Source.Line > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("substituted anonymous field lost its source declaration evidence")
	}
}

// Concrete type substitution cannot bypass the existing helper recursion budget.
func TestGenericHelperRecursionBudget(t *testing.T) {
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: "../testdata/helpergeneric", Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{genericFrontend()}, MaxDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range result.Bundle.Index() {
		if !strings.HasSuffix(entry.Symbol, ".HRecursive") {
			continue
		}
		found = true
		_, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Recursion", Version: "1"})
		if err == nil || !strings.Contains(err.Error(), "depth budget") {
			t.Fatalf("unbounded recursion accepted: %v", err)
		}
	}
	if !found {
		t.Fatal("recursive candidate missing")
	}
}

// Exhaust the substitution budget through a real generic source graph, without executing its large payload.
func TestGenericHelperTypeBudget(t *testing.T) {
	dir := t.TempDir()
	var source strings.Builder
	source.WriteString("package sample\nfunc expand[T any]() any { var value struct {\n")
	for i := 0; i < 4100; i++ {
		fmt.Fprintf(&source, "Field%d [%d]T\n", i, i)
	}
	source.WriteString("}; return value }\nfunc HLarge() any { return expand[byte]() }\n")
	for name, content := range map[string]string{"go.mod": "module example.test/generic-budget\n\ngo 1.27.1\n", "app.go": source.String()} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{genericFrontend()}})
	if err != nil {
		t.Fatal(err)
	}
	index := result.Bundle.Index()
	if len(index) != 1 {
		t.Fatalf("expected one large source candidate, got %d", len(index))
	}
	_, err = openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/value", OperationKey: index[0].Key}}, openapi.Config{Title: "Type budget", Version: "1"})
	if err == nil || !strings.Contains(err.Error(), "generic helper substitution exceeds the 4096-type budget") {
		t.Fatalf("type substitution exhaustion was not diagnosed: %v", err)
	}
}
