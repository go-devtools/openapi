package consumer

import (
	"context"
	. "github.com/openapi-golang/openapi/compiler"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/spec"
)

// 验证调用分支、元组和短路求值保留各自的响应提交状态。
// Verify call branches, tuples, and short-circuit evaluation preserve their response commits.
func TestCallFlowEvaluation(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
type Channel struct{}
func (*Channel) Commit(int) {}
func (*Channel) Write(int) {}
func choose(c *Channel, flag bool) int { if flag {c.Commit(409); return 209}; return 201 }
func pair(flag bool) (int, bool) { if flag {return 201,true}; return 202,false }
func clean() error {return nil}
func commit(c *Channel) bool {c.Commit(409);return true}
func receiver(c *Channel) *Channel {c.Commit(401);return c}
func argument(c *Channel) int {c.Commit(402);return 200}
type Failure struct{}
func (*Failure) Error() string {return "failure"}
func boxed() error {var err *Failure;return err}
func set(code *int){*code=201}
func HBoxed(c *Channel){if boxed()==nil{c.Write(500)}else{c.Write(200)}}
func HMutation(c *Channel){code:=200;alias:=&code;set(alias);c.Write(code)}
func HHelper(c *Channel, flag bool) {c.Write(choose(c, flag))}
func HTuple(c *Channel, flag bool) {code, ok := pair(flag); if ok {c.Write(code)} else {c.Write(204)}}
func HDecl(c *Channel, flag bool) {var code, ok = pair(flag); if ok {c.Write(code)} else {c.Write(204)}}
func HNil(c *Channel) {if clean()!=nil {c.Write(400);return};c.Write(200)}
func HShort(c *Channel, flag bool) {if flag && commit(c) {c.Write(201)} else {c.Write(202)}}
func HOr(c *Channel, flag bool) {if flag || commit(c) {c.Write(201)}}
func HOrder(c *Channel) {receiver(c).Write(argument(c))}
func HSwitch(c *Channel, flag bool) {switch code,_ := pair(flag);code {case 201:c.Write(201);case 202:c.Write(202);default:c.Write(500)}}
`
	for name, body := range map[string]string{"go.mod": "module example.test/calls\n\ngo 1.27.1\n", "app.go": source} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	front := Frontend{Name: "neutral-call-flow-v1", Match: func(f Function) bool { return strings.HasPrefix(f.Object.Name(), "H") }, Call: func(c CallContext) ([]Effect, error) {
		if c.Object == nil {
			return nil, nil
		}
		switch c.Object.Name() {
		case "Commit", "Write":
			status := ""
			if len(c.Arguments) > 0 && c.Arguments[0].Constant != nil {
				status = c.Arguments[0].Constant.ExactString()
			}
			effect := Effect{Kind: ResponseCommit, Status: status, Source: c.Source}
			if c.Object.Name() == "Write" {
				effect.Kind = ResponseBody
				effect.MediaType = "text/plain"
				effect.WireSchema = spec.Typed("string")
			}
			return []Effect{effect}, nil
		}
		return nil, nil
	}}
	options := Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{front}}
	result, err := Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"HBoxed": {"200"}, "HMutation": {"201"}, "HHelper": {"201", "409"}, "HTuple": {"201", "204"}, "HDecl": {"201", "204"}, "HNil": {"200"}, "HShort": {"202", "409"}, "HOr": {"201", "409"}, "HOrder": {"401"}, "HSwitch": {"201", "202"}}
	if len(result.Bundle.Index()) != len(want) {
		t.Fatal("missing handlers")
	}
	for _, entry := range result.Bundle.Index() {
		name := strings.TrimPrefix(entry.Symbol, "example.test/calls.")
		t.Run(name, func(t *testing.T) {
			if _, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Calls", Version: "1"}); err != nil {
				t.Fatal(err)
			}
			var got []string
			for code := range entry.Operation.Responses {
				got = append(got, code)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, want[name]) {
				t.Fatalf("responses %v, want %v", got, want[name])
			}
		})
	}
	options.MaxPaths = 1
	limited, err := Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range limited.Bundle.Index() {
		if strings.HasSuffix(entry.Symbol, ".HHelper") {
			if _, err := openapi.Build(limited.Bundle, []openapi.Route{{Method: "POST", Path: "/value", OperationKey: entry.Key}}, openapi.Config{Title: "Calls", Version: "1"}); err == nil {
				t.Fatal("truncated call paths were accepted")
			}
		}
	}
}
