// Generate a portable Bundle with only the public compiler SDK.
package main

import (
	"context"
	"os"

	"github.com/openapi-golang/openapi/compiler"
)

// Compile actual source with a return-value frontend without executing the handler.
func main() {
	frontend := compiler.Frontend{
		Name:  "compatibility-return-v1",
		Match: func(function compiler.Function) bool { return function.Object.Name() == "Create" },
		Entry: func(function compiler.Function) []compiler.Effect {
			return []compiler.Effect{{Kind: compiler.RequestBody, MediaType: "application/json", Required: true, Payload: compiler.Value{Type: function.Signature.Params().At(0).Type()}, Source: function.Source}}
		},
		Return: func(result compiler.ReturnContext) ([]compiler.Effect, error) {
			return []compiler.Effect{{Kind: compiler.ResponseBody, Status: "201", MediaType: "application/json", Payload: result.Values[0], Source: result.Source}}, nil
		},
	}
	result, err := compiler.Compile(context.Background(), compiler.Options{
		Load:      compiler.LoadOptions{Dir: "./api"},
		Frontends: []compiler.Frontend{frontend},
	})
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(os.Args[1], result.Bundle.JSON(), 0600); err != nil {
		panic(err)
	}
}
