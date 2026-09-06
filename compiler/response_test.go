package compiler

import (
	"context"
	"encoding/json"
	"go/constant"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
)

// Verify unknown commits, header alternatives, and explicit wire schemas without a framework dependency.
func TestResponseStateWithoutFramework(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
type Channel struct{}
func (*Channel) Header(string, string) {}
func (*Channel) Fallback(string, string) {}
func (*Channel) Commit(int) {}
func (*Channel) Write(int) {}
func HUnknown(c *Channel, code int) { c.Commit(code); c.Write(200) }
func HEmpty(c *Channel) { c.Header("X-Value", ""); c.Fallback("X-Value", "fallback"); c.Write(200) }
func HHeaders(c *Channel, left bool) {
 if left { c.Header("X-Value", "left") } else { c.Header("X-Value", "right") }
 c.Write(200)
}
func HInvalid(c *Channel) { c.Header("Invalid@Header", "value"); c.Write(200) }
func HNoContent(c *Channel) { c.Write(204) }
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/response\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	wire := spec.Typed("string")
	wireBefore, _ := json.Marshal(wire)
	front := Frontend{Name: "neutral-response", Match: func(f Function) bool { return f.Signature.Recv() == nil && strings.HasPrefix(f.Object.Name(), "H") }, Call: func(c CallContext) ([]Effect, error) {
		if c.Object == nil {
			return nil, nil
		}
		status := ""
		if len(c.Arguments) > 0 && c.Arguments[0].Constant != nil && c.Arguments[0].Constant.Kind() == constant.Int {
			status = c.Arguments[0].Constant.ExactString()
		}
		source := c.Source
		source.Kind, source.Rule = "derived", "neutral."+c.Object.Name()
		switch c.Object.Name() {
		case "Header", "Fallback":
			return []Effect{{Kind: ResponseHeader, Name: constant.StringVal(c.Arguments[0].Constant), Payload: c.Arguments[1], HeaderIfEmpty: c.Object.Name() == "Fallback", Source: source}}, nil
		case "Commit":
			return []Effect{{Kind: ResponseCommit, Status: status, Source: source}}, nil
		case "Write":
			return []Effect{{Kind: ResponseBody, Status: status, MediaType: "text/plain", WireSchema: wire, Payload: Value{Type: types.Typ[types.String]}, Source: source}}, nil
		}
		return nil, nil
	}}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{front}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Bundle.Index() {
		t.Run(entry.Symbol, func(t *testing.T) {
			doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Neutral response", Version: "1"})
			if strings.HasSuffix(entry.Symbol, ".HUnknown") || strings.HasSuffix(entry.Symbol, ".HInvalid") {
				if err == nil {
					t.Fatal("invalid header or unknown committed status accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(entry.Symbol, ".HNoContent") {
				if len(entry.Operation.Responses["204"].Value.Content) != 0 {
					t.Fatal("bodyless status gained content")
				}
				return
			}
			validator, err := contracttest.Compile(doc.JSON(), "/paths/~1value/get/responses/200/headers/X-Value/schema", contracttest.Options{})
			if err != nil {
				t.Fatal(err)
			}
			values := []string{"left", "right"}
			if strings.HasSuffix(entry.Symbol, ".HEmpty") {
				values = []string{"fallback"}
			}
			for _, value := range values {
				if err := validator.Value(value); err != nil {
					t.Fatal(err)
				}
			}
			if validator.Value("unrelated") == nil {
				t.Fatal("header alternatives lost their constraints")
			}
			found := false
			for _, fact := range doc.Report().Facts {
				found = found || strings.HasPrefix(fact.Rule, "neutral.Header") || strings.HasPrefix(fact.Rule, "neutral.Fallback")
			}
			if !found {
				t.Fatal("header provenance was dropped")
			}
		})
	}
	wireAfter, _ := json.Marshal(wire)
	if string(wireBefore) != string(wireAfter) {
		t.Fatal("frontend wire schema was modified")
	}
}
