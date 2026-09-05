package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi"
)

// 从标准库函数返回值复用同一编译管线，证明 SDK 不预设框架 context。
func TestReturnValueFrontend(t *testing.T) {
	dir := t.TempDir()
	source := `// 测试期前端的真实业务输入。
package sample

// 创建信息。
type Request struct { Name string }
// 响应信息。
type Response struct { Name string }
// 创建用户
//
// 返回创建结果。
func Create(req Request) (Response, error) { return Response{Name:req.Name}, nil }
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/sample\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	frontend := Frontend{Name: "test-return-v1", Match: func(f Function) bool { return f.Signature.Results().Len() == 2 },
		Entry: func(f Function) []Effect {
			return []Effect{{Kind: RequestBody, MediaType: "application/json", Payload: Value{Type: f.Signature.Params().At(0).Type()}, Source: f.Source}}
		},
		Return: func(c ReturnContext) ([]Effect, error) {
			return []Effect{{Kind: ResponseBody, Status: "201", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
		},
	}
	result, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir}, Frontends: []Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	index := result.Bundle.Index()
	if len(index) != 1 {
		t.Fatalf("候选数量错误：%+v", index)
	}
	doc, err := openapi.Build(result.Bundle, []openapi.Route{{Method: "POST", Path: "/users", OperationKey: index[0].Key}}, openapi.Config{Title: "用户", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc.JSON()), "创建用户") || !strings.Contains(string(doc.JSON()), `"201"`) {
		t.Fatalf("未复用注释与返回值：%s", doc.JSON())
	}
	output := filepath.Join(dir, "internal", "apidoc")
	if err = result.Write(WriteOptions{Dir: output, Package: "apidoc"}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(output, "zz_openapi.gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(first), "example.com/sample\"") || strings.Contains(string(first), "openapi/compiler\"") {
		t.Fatal("生成文件依赖业务或编译器")
	}
	if err = result.Check(WriteOptions{Dir: output, Package: "apidoc"}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "sample.go"), []byte(strings.Replace(source, "创建用户", "登记用户", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := Compile(context.Background(), Options{Load: LoadOptions{Dir: dir}, Frontends: []Frontend{frontend}})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Check(WriteOptions{Dir: output, Package: "apidoc"}) == nil {
		t.Fatal("源码变更未使生成物过期")
	}
}
