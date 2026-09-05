# Call outcomes and control flow

A generation frontend can supply `Frontend.CallOutcomes` to describe a finite set of alternatives for one actual Go call. Each `CallOutcome` pairs a `Results` tuple with the `Effects` that occur on the same path. This is a generation-only SDK: callbacks, `go/types` objects, and analysis state are never serialized into the runtime Bundle.

An empty alternative set falls back to the existing `Call` callback and source-helper analysis. A callback error, an incorrect result count, contradictory nil facts, or an exceeded path budget produces a diagnostic. The result tuple must have exactly the call signature's arity; omit an individual Value.Type to use its static result type. `Nil` and `NonNil` describe known Go nil comparisons. Leave both unset when nullness is unknown. They do not authorize guessing an unknown interface payload's concrete wire type.

```go
// 成功不写响应；错误立即提交状态，但不结束当前 Go 函数。
// Success does not write a response; failure commits a status without ending the current Go function.
outcomes := []compiler.CallOutcome{
    {Results: []compiler.Value{{Nil: true}}},
    {
        Results: []compiler.Value{{NonNil: true}},
        Effects: []compiler.Effect{
            {Kind: compiler.ResponseCommit, Status: "409", Source: source},
            {Kind: compiler.Abort, Source: source},
        },
    },
}
```

The core evaluates each alternative independently through subsequent assignments, declarations, if/switch conditions, returns, and helper calls. Ignoring an error does not erase its effects. An error branch that writes status 422 after the example's commit still has wire status 409. Returning immediately after failure leaves the committed response bodyless.

Method receivers are evaluated before call arguments. Expressions and arguments retain Go's left-to-right call order, including unknown short-circuit conditions. Known nil and boolean results prune impossible branches. Bounded source helpers can return multiple correlated paths and tuples; their local return does not terminate the caller. A typed nil pointer boxed in an interface is distinct from a nil interface. Simple local pointer aliases preserve helper writes; calls governed by frontend rules or unknown external calls invalidate facts about addresses they may mutate.

MaxPaths applies within expressions and call alternatives as well as statement paths. MaxCalls and MaxDepth remain shared analysis limits. Truncation is reported and blocks document construction for an included operation. This implementation does not claim complete heap alias analysis, higher-order closures, arbitrary pointer/field mutation, generic specialization, or a cached parameterized-helper summary engine. Unsupported operations and the remaining full-goal matrix still need explicit work.

Tests exercise a neutral carrier, public SDK consumers outside the core module, ignored and checked errors, malformed callback results, helper tuples, switch and short-circuit branches, pointer aliases, boxed nil values, and receiver/argument write ordering. Framework-specific error statuses and codec selection belong to adapters.

`CallOutcome.When` optionally restricts an alternative to a finite request domain. See [request conditions](request-conditions.md) for generation, Bundle compatibility, linking, and provenance.
