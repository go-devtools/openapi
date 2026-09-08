package compiler_test

import (
	"context"
	"encoding/json"
	"go/constant"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	core "github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
)

// Drive neutral decoding outcomes through real Go branches and returned HTTP statuses.
func TestRequestNonEmptyBodyProof(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/body-presence\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := `package presence
// Decode is supplied by the neutral frontend.
func Decode() error { return nil }
// Require a valid nonempty document before accepting the request.
func HChecked(flag bool) int { if err := Decode(); err != nil { return 400 }; return 201 }
// Continue successfully after any decoding result.
func HIgnored(flag bool) int { Decode(); return 200 }
// Redirect only after decoding succeeds.
func HRedirect(flag bool) int { if err := Decode(); err != nil { return 400 }; return 303 }
// Keep a successful path that never reads the body.
func HSkip(flag bool) int { if flag { return 202 }; return HChecked(flag) }
// A later method test refines successful paths while the earlier failure remains unconditional.
func HConditional(flag bool) int { if err := Decode(); err != nil { return 400 }; if IsPost() { return 201 }; return 202 }
// The frontend supplies the actual method test.
func IsPost() bool { return true }
// No accepted response provides a body-presence proof.
func HErrorOnly(flag bool) int { Decode(); return 400 }
`
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	frontend := core.Frontend{Name: "neutral-body-presence-v1", Match: func(f core.Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, CallOutcomes: func(c core.CallContext) ([]core.CallOutcome, error) {
		if c.Object != nil && c.Object.FullName() == "example.test/body-presence.IsPost" {
			return []core.CallOutcome{{When: openapi.RequestCondition{Methods: []string{"POST"}}, Results: []core.Value{{Constant: constant.MakeBool(true)}}}, {When: openapi.RequestCondition{ExceptMethods: []string{"POST"}}, Results: []core.Value{{Constant: constant.MakeBool(false)}}}}, nil
		}
		if c.Object == nil || c.Object.FullName() != "example.test/body-presence.Decode" {
			return nil, nil
		}
		schema := spec.Typed("object")
		schema.Properties = map[string]*spec.Schema{"Name": spec.Typed("string")}
		schema.Required = spec.Set([]string{"Name"})
		effect := core.Effect{Kind: core.RequestBody, MediaType: "application/json", WireSchema: schema, Source: c.Source}
		success := effect
		success.NonEmptyBody = true
		return []core.CallOutcome{{Results: []core.Value{{Nil: true}}, Effects: []core.Effect{success}}, {Results: []core.Value{{NonNil: true}}, Effects: []core.Effect{effect}}}, nil
	}, Return: func(c core.ReturnContext) ([]core.Effect, error) {
		return []core.Effect{{Kind: core.ResponseStatus, Status: c.Values[0].Constant.ExactString(), Source: c.Source}}, nil
	}}
	for _, disabled := range []bool{false, true} {
		result, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{frontend}, DisableHelperSummaries: disabled})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Bundle.Index() {
			doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/value", OperationKey: item.Key}}, openapi.Config{Title: "Neutral body presence", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			want := strings.HasSuffix(item.Symbol, ".HChecked") || strings.HasSuffix(item.Symbol, ".HRedirect") || strings.HasSuffix(item.Symbol, ".HConditional")
			var parsed spec.OpenAPI
			if err := json.Unmarshal(doc.JSON(), &parsed); err != nil {
				t.Fatal(err)
			}
			body := parsed.Paths["/value"].Post.RequestBody
			if body == nil || body.Value.Required.Value != want {
				t.Fatalf("%s disabled=%t: %s", item.Symbol, disabled, doc.JSON())
			}
		}
	}
	// A proof attached to a response cannot be trusted as a request-body constraint.
	original := frontend.CallOutcomes
	frontend.CallOutcomes = func(c core.CallContext) ([]core.CallOutcome, error) {
		outcomes, err := original(c)
		if len(outcomes) > 0 && len(outcomes[0].Effects) > 0 {
			outcomes[0].Effects[0].Kind = core.ResponseStatus
			outcomes[0].Effects[0].Status = "204"
		}
		return outcomes, err
	}
	invalid, err := core.Compile(context.Background(), core.Options{Load: core.LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []core.Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = openapi.Build(invalid.Bundle, []openapi.Route{{Method: "POST", Path: "/value", OperationKey: "example.test/body-presence.HChecked"}}, openapi.Config{Title: "Invalid proof", Version: "1"})
	if err == nil || !strings.Contains(err.Error(), "nonempty body proof requires") {
		t.Fatalf("invalid proof accepted: %v", err)
	}

}
