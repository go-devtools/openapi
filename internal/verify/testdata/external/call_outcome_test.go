package consumer

import (
	"context"
	. "github.com/openapi-golang/openapi/compiler"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// 验证公开调用备选将返回值和写入效果关联，忽略返回值也不能丢失提交。
// Verify public call outcomes correlate values with writes even when callers ignore results.
func TestFrontendCallOutcomes(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
type Channel struct{}
func (*Channel) Try() error {return nil}
func (*Channel) Bad() error {return nil}
func (*Channel) Write(int) {}
func HChecked(c *Channel){err:=c.Try();if err!=nil{c.Write(422);return};c.Write(201)}
func HIgnored(c *Channel){c.Try();c.Write(201)}
func HReturn(c *Channel){if c.Try()!=nil{return};c.Write(201)}
func HMalformed(c *Channel){c.Bad();c.Write(200)}
`
	for name, body := range map[string]string{"go.mod": "module example.test/outcomes\n\ngo 1.27.1\n", "app.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	front := Frontend{Name: "neutral-outcomes-v1", Match: func(f Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, CallOutcomes: func(c CallContext) ([]CallOutcome, error) {
		if c.Object == nil {
			return nil, nil
		}
		if c.Object.Name() == "Bad" {
			return []CallOutcome{{}}, nil
		}
		if c.Object.Name() != "Try" {
			return nil, nil
		}
		return []CallOutcome{
			{Results: []Value{{Nil: true}}},
			{Results: []Value{{NonNil: true}}, Effects: []Effect{{Kind: ResponseCommit, Status: "409", Source: c.Source}, {Kind: Abort, Source: c.Source}}},
		}, nil
	}, Call: func(c CallContext) ([]Effect, error) {
		if c.Object != nil && c.Object.Name() == "Write" {
			return []Effect{{Kind: ResponseBody, Status: c.Arguments[0].Constant.ExactString(), MediaType: "text/plain", WireSchema: spec.Typed("string"), Payload: Value{Type: types.Typ[types.String]}, Source: c.Source}}, nil
		}
		return nil, nil
	}}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{front}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Bundle.Index() {
		t.Run(entry.Symbol, func(t *testing.T) {
			_, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Outcomes", Version: "1"})
			if strings.HasSuffix(entry.Symbol, ".HMalformed") {
				if err == nil {
					t.Fatal("malformed frontend result accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var statuses []string
			for status := range entry.Operation.Responses {
				statuses = append(statuses, status)
			}
			sort.Strings(statuses)
			if !reflect.DeepEqual(statuses, []string{"201", "409"}) {
				t.Fatalf("uncorrelated call effects: %v", statuses)
			}
			empty := len(entry.Operation.Responses["409"].Value.Content) == 0
			if empty != strings.HasSuffix(entry.Symbol, ".HReturn") {
				t.Fatal("abort incorrectly ended the Go function or return was ignored")
			}
		})
	}
}
