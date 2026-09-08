package compiler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"go/constant"
	"go/types"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/testdata/helpersummary"
)

// Register neutral protocol rules by complete package and receiver identity, with test-only observation counts.
func summaryFrontend(counts map[string]int) core.Frontend {
	return core.Frontend{Name: "neutral-helper-summary-v1", Match: func(f core.Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, Entry: func(f core.Function) []core.Effect {
		return []core.Effect{{Kind: core.ResponseStatus, Status: "200", Source: f.Source}}
	}, CallOutcomes: func(c core.CallContext) ([]core.CallOutcome, error) {
		if c.Object == nil || c.Object.FullName() != "(*github.com/openapi-golang/openapi/testdata/helpersummary.Channel).IsGet" {
			return nil, nil
		}
		counts[c.Function.Symbol+"/IsGet"]++
		return []core.CallOutcome{{When: openapi.RequestCondition{Methods: []string{"GET"}}, Results: []core.Value{{Type: types.Typ[types.Bool], Constant: constant.MakeBool(true)}}}, {When: openapi.RequestCondition{ExceptMethods: []string{"GET"}}, Results: []core.Value{{Type: types.Typ[types.Bool], Constant: constant.MakeBool(false)}}}}, nil
	}, Call: func(c core.CallContext) ([]core.Effect, error) {
		if c.Object == nil || c.Object.Pkg() == nil || c.Object.Pkg().Path() != "github.com/openapi-golang/openapi/testdata/helpersummary" || !strings.HasPrefix(c.Object.FullName(), "(*github.com/openapi-golang/openapi/testdata/helpersummary.Channel).") {
			return nil, nil
		}
		counts[c.Function.Symbol+"/"+c.Object.Name()]++
		source := c.Source
		source.Kind = "derived"
		source.Rule = "neutral." + c.Object.Name()
		status := ""
		if len(c.Arguments) > 0 && c.Arguments[0].Constant != nil {
			status = c.Arguments[0].Constant.ExactString()
		}
		switch c.Object.Name() {
		case "Send":
			return []core.Effect{{Kind: core.ResponseHeader, Name: "Content-Type", Payload: core.Value{Type: types.Typ[types.String], Constant: constant.MakeString("application/json")}, Source: source}, {Kind: core.ResponseBody, Status: status, MediaType: "application/json", Payload: c.Arguments[1], Source: source}}, nil
		case "Status":
			return []core.Effect{{Kind: core.ResponseStatus, Status: status, Source: source}}, nil
		case "Commit":
			return []core.Effect{{Kind: core.ResponseCommit, Status: status, Source: source}}, nil
		case "Header":
			name, value := constant.StringVal(c.Arguments[0].Constant), constant.StringVal(c.Arguments[1].Constant)
			return []core.Effect{{Kind: core.ResponseHeader, Name: name, Payload: c.Arguments[1], DeleteHeader: value == "", Source: source}}, nil
		case "Trace":
			return []core.Effect{{Kind: core.Handled}}, nil
		}
		return nil, nil
	}}
}

// Compile every source candidate while keeping invocation observations separate from contract behavior.
func summaryCompile(t *testing.T, disable bool, configure func(*core.Options)) (*core.Result, map[string]int) {
	t.Helper()
	counts := map[string]int{}
	options := core.Options{Load: core.LoadOptions{Dir: "../testdata/helpersummary", Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{summaryFrontend(counts)}, DisableHelperSummaries: disable, Explain: true}
	if configure != nil {
		configure(&options)
	}
	result, err := core.Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	return result, counts
}

// Select one actual source operation without allowing unrelated diagnostics to contaminate its document.
func summaryDocument(result *core.Result, name, method string) (*openapi.Document, error) {
	for _, entry := range result.Bundle.Index() {
		if strings.HasSuffix(entry.Symbol, "."+name) {
			return openapi.Build(result.Bundle, []openapi.Route{{Method: method, Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Helper summaries", Version: "1"})
		}
	}
	panic("missing source candidate " + name)
}

// Cached and uncached execution must produce identical documents and validate real status, headers, and body bytes.
func TestHelperSummaryWireEquivalence(t *testing.T) {
	cached, counts := summaryCompile(t, false, nil)
	plain, uncachedCounts := summaryCompile(t, true, nil)
	for _, sample := range []struct {
		name  string
		call  func(*helpersummary.Channel, bool)
		codes [2]int
	}{
		{"HReplay", helpersummary.HReplay, [2]int{201, 201}},
		{"HArguments", helpersummary.HArguments, [2]int{202, 201}},
		{"HReturn", helpersummary.HReturn, [2]int{201, 201}},
		{"HCommit", helpersummary.HCommit, [2]int{409, 409}},
		{"HState", helpersummary.HState, [2]int{203, 202}},
		{"HMutation", helpersummary.HMutation, [2]int{202, 202}},
		{"HFactory", helpersummary.HFactory, [2]int{402, 402}},
		{"HDepth", helpersummary.HDepth, [2]int{201, 201}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			doc, err := summaryDocument(cached, sample.name, "GET")
			if err != nil {
				t.Fatal(err)
			}
			reference, err := summaryDocument(plain, sample.name, "GET")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(doc.JSON(), reference.JSON()) {
				t.Fatal("summary replay changed the contract")
			}
			for i, flag := range []bool{false, true} {
				recorder := httptest.NewRecorder()
				sample.call(helpersummary.NewChannel(recorder, "GET"), flag)
				response := recorder.Result()
				raw, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != sample.codes[i] || response.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("unexpected wire response: %d %v", response.StatusCode, response.Header)
				}
				code, _ := json.Marshal(response.StatusCode)
				validator, err := contracttest.Compile(doc.JSON(), "/paths/~1value/get/responses/"+string(code)+"/content/application~1json/schema", contracttest.Options{})
				if err != nil {
					t.Fatal(err)
				}
				if err = validator.JSON(raw); err != nil {
					t.Fatal(err)
				}
				if sample.name == "HCommit" && response.Header.Get("X-State") != "before" {
					t.Fatal("late header replaced the committed header")
				}
			}
			explanation, err := cached.Explain(core.ExplainQuery{Symbol: "github.com/openapi-golang/openapi/testdata/helpersummary." + sample.name})
			if err != nil {
				t.Fatal(err)
			}
			original, err := plain.Explain(core.ExplainQuery{Symbol: explanation.Symbol})
			if err != nil {
				t.Fatal(err)
			}
			first, _ := json.Marshal(explanation)
			second, _ := json.Marshal(original)
			if !bytes.Equal(first, second) {
				t.Fatal("summary replay changed source evidence")
			}
		})
	}
	prefix := "github.com/openapi-golang/openapi/testdata/helpersummary."
	for _, method := range []string{"deliver/Send", "choose/Trace", "committed/Commit", "leaf/Trace"} {
		if counts[prefix+method] >= uncachedCounts[prefix+method] {
			t.Errorf("effect summaries were not reused for %s: cached=%d uncached=%d", method, counts[prefix+method], uncachedCounts[prefix+method])
		}
	}
	if counts[prefix+"increment/Trace"] != uncachedCounts[prefix+"increment/Trace"] {
		t.Fatal("mutable caller pointers were memoized")
	}
	for _, result := range []*core.Result{cached, plain} {
		_, err := summaryDocument(result, "HSequence", "GET")
		if err == nil || !strings.Contains(err.Error(), "multiple bodies") {
			t.Fatalf("sequential writes became alternatives: %v", err)
		}
	}
}

// Correlated request conditions survive replay and route selection with actual method-dependent responses.
func TestHelperSummaryRequestConditions(t *testing.T) {
	cached, counts := summaryCompile(t, false, nil)
	plain, referenceCounts := summaryCompile(t, true, nil)
	for _, method := range []string{"GET", "POST"} {
		doc, err := summaryDocument(cached, "HCondition", method)
		if err != nil {
			t.Fatal(err)
		}
		reference, err := summaryDocument(plain, "HCondition", method)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(doc.JSON(), reference.JSON()) {
			t.Fatal("summary request domain changed")
		}
		for _, flag := range []bool{false, true} {
			recorder := httptest.NewRecorder()
			helpersummary.HCondition(helpersummary.NewChannel(recorder, method), flag)
			status := 201
			if method != "GET" {
				status = 202
			}
			if recorder.Code != status {
				t.Fatalf("actual status=%d", recorder.Code)
			}
			code, _ := json.Marshal(status)
			validator, err := contracttest.Compile(doc.JSON(), "/paths/~1value/"+strings.ToLower(method)+"/responses/"+string(code)+"/content/application~1json/schema", contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err = validator.JSON(recorder.Body.Bytes()); err != nil {
				t.Fatal(err)
			}
		}
	}
	key := "github.com/openapi-golang/openapi/testdata/helpersummary.conditional/IsGet"
	if counts[key] != 1 || referenceCounts[key] != 2 {
		t.Fatalf("conditional effects were not summarized: cached=%d uncached=%d", counts[key], referenceCounts[key])
	}
}

// Summary limits fail the selected operation without making unrelated source candidates unusable.
func TestHelperSummaryBudgets(t *testing.T) {
	result, _ := summaryCompile(t, false, func(options *core.Options) { options.MaxSummaries = 1 })
	if _, err := summaryDocument(result, "HReplay", "GET"); err != nil {
		t.Fatalf("equivalent contexts consumed multiple summaries: %v", err)
	}
	if _, err := summaryDocument(result, "HArguments", "GET"); err == nil || !strings.Contains(err.Error(), "summary count budget") {
		t.Fatalf("distinct contexts escaped the budget: %v", err)
	}
	tiny, _ := summaryCompile(t, false, func(options *core.Options) { options.MaxSummaryBytes = 64 })
	if _, err := summaryDocument(tiny, "HReplay", "GET"); err == nil || !strings.Contains(err.Error(), "summary byte budget") {
		t.Fatalf("summary byte limit was ignored: %v", err)
	}
	for _, disable := range []bool{false, true} {
		depth, _ := summaryCompile(t, disable, func(options *core.Options) { options.MaxDepth = 2 })
		if _, err := summaryDocument(depth, "HDepth", "GET"); err == nil || !strings.Contains(err.Error(), "depth budget") {
			t.Fatalf("cached shallow helper bypassed deeper call budget: %v", err)
		}

		limited, _ := summaryCompile(t, disable, func(options *core.Options) { options.MaxCalls = 3 })
		if _, err := summaryDocument(limited, "HReplay", "GET"); err == nil || !strings.Contains(err.Error(), "call analysis exceeds") {
			t.Fatalf("cache hit bypassed call budget: %v", err)
		}
	}
}

// Repeated cache-enabled compilation remains deterministic while budget settings participate in freshness.
func TestHelperSummaryDeterminism(t *testing.T) {
	first, _ := summaryCompile(t, false, nil)
	second, _ := summaryCompile(t, false, nil)
	left := first.Bundle.JSON()
	right := second.Bundle.JSON()
	if !bytes.Equal(left, right) {
		t.Fatal("summary identity allocation changed deterministic bundle bytes")
	}
	for _, configure := range []func(*core.Options){func(o *core.Options) { o.MaxSummaries = 513 }, func(o *core.Options) { o.MaxSummaryBytes = (16 << 20) + 1 }} {
		other, _ := summaryCompile(t, false, configure)
		raw := other.Bundle.JSON()
		if bytes.Equal(left, raw) {
			t.Fatal("summary budget did not participate in the source fingerprint")
		}
	}
	for _, options := range []core.Options{{MaxSummaries: -1}, {MaxSummaryBytes: -1}} {
		options.Load = core.LoadOptions{Dir: "../testdata/helpersummary", Env: []string{"GOWORK=off"}}
		options.Frontends = []core.Frontend{summaryFrontend(map[string]int{})}
		if _, err := core.Compile(context.Background(), options); err == nil || !strings.Contains(err.Error(), "all budgets must be positive") {
			t.Fatalf("invalid summary budget accepted: %v", err)
		}
	}
}
