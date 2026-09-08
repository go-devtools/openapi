# Call outcomes and control flow

A generation frontend can supply `Frontend.CallOutcomes` to describe a finite set of alternatives for one actual Go call. Each `CallOutcome` pairs a `Results` tuple with the `Effects` that occur on the same path. This is a generation-only SDK: callbacks, `go/types` objects, and analysis state are never serialized into the runtime Bundle.

An empty alternative set falls back to the existing `Call` callback and source-helper analysis. A callback error, an incorrect result count, contradictory nil facts, or an exceeded path budget produces a diagnostic. The result tuple must have exactly the call signature's arity; omit an individual Value.Type to use its static result type. `Nil` and `NonNil` describe known Go nil comparisons. Leave both unset when nullness is unknown. They do not authorize guessing an unknown interface payload's concrete wire type.

```go
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

MaxPaths applies within expressions and call alternatives as well as statement paths. MaxCalls and MaxDepth remain shared analysis limits. Truncation is reported and blocks document construction for an included operation. Known function values and captured cells use the [callback and function-value analyzer](callbacks.md). Complete heap alias analysis and arbitrary pointer mutation are outside its supported scope. Eligible helpers use the parameterized effect summaries described below. Unsupported effects remain diagnostics.

Tests exercise a neutral carrier, public SDK consumers outside the core module, ignored and checked errors, malformed callback results, helper tuples, switch and short-circuit branches, pointer aliases, boxed nil values, and receiver/argument write ordering. Framework-specific error statuses and codec selection belong to adapters.

`CallOutcome.When` optionally restricts an alternative to a finite request domain. See [request conditions](request-conditions.md) for generation, Bundle compatibility, linking, and provenance.

## Concrete generic helper calls

Source helper analysis retains the type arguments reported by `go/types`, including inferred instantiations, explicit instantiations with one or several arguments, function variables, aliases, returned closures, and instantiated receiver methods. Those arguments apply to local zero values, named results, conversions, anonymous fields, named generic payloads, and nested helper calls. A function value keeps its actual implementation and selected type arguments together.

For example, `wrap[T any](value T) Envelope[T]` called with a `User` produces the `Envelope[User]` wire contract. Another branch calling it with a string keeps a separate string payload. A source helper can forward a constant status and the concrete payload to a frontend-recognized response call. Analysis never invokes the business helper or a DTO codec.

Each call frame owns its substitution cache; loaded declarations and `types.Info` remain unchanged. Substituted anonymous fields resolve comments and source evidence through their original declaration objects. Generic receiver calls resolve the original function object before analyzing its source body. Uninstantiated generic handler candidates still retain diagnostics for unresolved critical payloads; a constraint such as `any` or `~int` is not itself a concrete runtime type.

Substitution is bounded to 4096 visited types per helper frame and shares the existing call, depth, path, and callback-iteration limits. Hitting a limit yields an unresolved diagnostic for an included operation. Parameterized effect summaries retain those concrete substitutions as part of their invocation context.

The public SDK tests compare real helper results and emitted status/payload pairs with independently validated schemas. Negative instances cover incorrect generic members, lost field constraints, conflicting branch types, and the distinction between nil pointers/collections and a fixed empty array. Additional tests check anonymous-field provenance and refusal when a recursive helper exceeds the configured depth.


## Parameterized helper effect summaries

The analyzer records a helper's correlated return tuples and protocol transitions for each distinct abstract invocation context. A context includes the original Go function object, concrete receiver and argument types, exact constant values, known fields and nil facts, generic type arguments, finite request conditions, and pending/committed response state. It uses actual Go identities rather than short function or type names. Distinct statuses, payload instantiations, or response states cannot share the same summary.

Reusing a summary preserves effect order, response commits, current versus committed headers, helper source locations, and request/result correlation. Receiver and argument expressions still run through static analysis in Go order on every call. The cache does not restore another invocation's local variables. Consecutive response bodies still produce an error; a cached alternative is not an excuse to treat sequential writes as mutually exclusive.

Frontend callbacks, codecs, and schema transforms must be deterministic for their inputs and declared `Configuration`. Do not make their contract output depend on an invocation counter or mutate retained rule configuration during compilation. A valid summary can reduce repeated frontend callback invocations. Business helpers and DTO codecs are still never executed by generation.

Calls carrying tracked addresses, mutable captures, or function values use ordinary bounded source analysis. Results containing addresses/captures, changed caller cells, and newly diagnosed paths are not retained as successful summaries. Those cases are not silently converted to effect-free calls. Captured callback analysis keeps its existing cell and iteration semantics.

Each candidate has its own cache, so a complex unselected operation cannot consume the summary budget of another operation. Cache hits still charge the original nested-call cost and honor the deepest helper frame. An insufficient call/depth budget leads through the existing diagnostic path instead of making a cached call appear cheaper for correctness checks.

| Option or bound | Default | Meaning |
| --- | --- | --- |
| `MaxSummaries` | 512 | Distinct admitted helper contexts per candidate, including contexts whose result cannot safely be reused |
| `MaxSummaryBytes` | 16 MiB | Normalized retained context keys, result values, effects, and response snapshots per candidate |
| Summary fact traversal | 4096 values, depth 64 | Bound each parameter context and complete retained result set |
| `DisableHelperSummaries` | false | Reanalyze each helper call for differential verification or troubleshooting |

All public summary settings participate in the source fingerprint. Byte accounting bounds normalized retained data; it is not an exact process-heap measurement and does not duplicate the size of already loaded Go type graphs or immutable registered rule implementations. Exceeding count, byte, or fact traversal limits yields a selected-operation diagnostic. It does not emit a truncated successful contract.

Tests compare cache-enabled and cache-disabled documents and complete Explain records, validate real HTTP statuses/headers/JSON, and confirm that only repeated analysis is reduced. They cover generic payload and constant substitution, committed and observed headers, returns, method-conditioned alternatives, mutable pointer and closure fallback, sequential-write diagnostics, per-candidate count/byte limits, call-cost accounting, deeper-call refusal, and deterministic regeneration.
