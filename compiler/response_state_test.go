package compiler

import (
	"context"
	"github.com/openapi-golang/openapi"
	"go/constant"
	"go/types"
	"os"
	"path/filepath"
	"testing"
)

// Frontends observe path-local pending status and headers without mutating analyzer state through snapshots.
// 前端按当前路径观察待提交状态和响应头，修改观察值不会写回分析器。
func TestFrontendResponseSnapshot(t *testing.T) {
	dir := t.TempDir()
	for name, raw := range map[string]string{"go.mod": "module example.test/response-state\n\ngo 1.27.1\n", "app.go": "package response\nfunc Set(){}\nfunc Observe(){}\nfunc Commit(){}\nfunc Late(){}\nfunc Verify(){}\nfunc Handle(){Set();Observe();Commit();Late();Verify()}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	seen := 0
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir, Env: []string{"GOWORK=off"}}, Frontends: []Frontend{{Name: "neutral-response-state", Match: func(f Function) bool { return f.Object.Name() == "Handle" }, Call: func(c CallContext) ([]Effect, error) {
		switch c.Object.Name() {
		case "Set":
			return []Effect{{Kind: ResponseStatus, Status: "202", Source: c.Source}, {Kind: ResponseHeader, Name: "X-Unknown", Payload: Value{Type: types.Typ[types.String]}, Source: c.Source}, {Kind: ResponseHeader, Name: "X-Mode", Payload: Value{Constant: constant.MakeString("real")}, Source: c.Source}}, nil
		case "Observe":
			seen++
			if len(c.Response.CommittedHeaders) != 0 {
				t.Fatal("pending headers appeared committed")
			}
			if c.Response.Status != "202" || c.Response.Committed || c.Response.Headers["X-Mode"].Value != "real" || !c.Response.Headers["X-Mode"].Known {
				t.Fatalf("bad pending snapshot: %#v", c.Response)
			}
			if header, exists := c.Response.Headers["X-Unknown"]; !exists || header.Known {
				t.Fatal("unknown header presence was lost")
			}
			delete(c.Response.Headers, "X-Mode")
		case "Commit":
			return []Effect{{Kind: ResponseCommit, Status: "203", Source: c.Source}}, nil
		case "Late":
			return []Effect{{Kind: ResponseHeader, Name: "X-Mode", Payload: Value{Constant: constant.MakeString("late")}, Source: c.Source}}, nil
		case "Verify":
			seen++
			if c.Response.CommittedHeaders["X-Mode"].Value != "real" || !c.Response.CommittedHeaders["X-Mode"].Known {
				t.Fatal("committed header snapshot was lost")
			}
			delete(c.Response.CommittedHeaders, "X-Mode")
			if c.Response.Status != "203" || !c.Response.Committed || c.Response.Headers["X-Mode"].Value != "late" {
				t.Fatalf("snapshot mutation escaped: %#v", c.Response)
			}
		}
		return []Effect{{Kind: Handled, Source: c.Source}}, nil
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bundle.Index()[0].Operation.Responses["203"].Value.Headers["X-Mode"].Value.Schema.Const.Value != "real" {
		t.Fatal("late header replaced committed wire value")
	}
	if seen != 2 {
		t.Fatalf("observations: %d", seen)
	}
	if _, err = openapi.Build(result.Bundle, []openapi.Route{{Method: "GET", Path: "/snapshot", OperationKey: result.Bundle.Index()[0].Key}}, openapi.Config{Title: "Snapshot", Version: "1"}); err != nil {
		t.Fatal(err)
	}
}
