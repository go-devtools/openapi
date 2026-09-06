# Synchronous callback analysis

The public compiler propagates known function values, bound methods and literal closures without executing application code. Each invocation allocates distinct lexical cells; aliases of one closure share its captures, while separate factory invocations retain separate cells. Branches own independent cell contents. Returning from a helper restores caller bindings while retaining reachable captured cells.

The current regressions cover capture reads at invocation time, factory instances, alias mutation, higher-order helpers, branch isolation, value/pointer method receivers, method expressions, frontend-recognized method aliases, unused closures and explicit diagnostics for nil/unknown/recursive calls. Direct field writes through known local identities are propagated; unresolved or nested write targets are diagnosed. Integer increments and decrements use the actual target's `Package.Sizes`, including signed wraparound. Floating-point increments retain an abstract typed value when exact rounding has not been established.

## Frontend.Callback

A frontend may return a `CallbackPlan` from `Frontend.Callback`. This rule precedes CallOutcomes and Call. Returning nil defers to those existing rules. The plan's Function is a propagated source function value, Arguments supplies the actual invocation arguments, and After contains neutral effects that occur after every callback return. Results describes the surrounding call's normal result tuple. No function, AST, capture address or framework object is serialized into Bundle.

An optional CallbackRepeat selects a boolean callback result position and the value that causes another invocation. MayInterrupt allows a separate exit before each invocation, with InterruptResults identifying the surrounding call's result on that path. Argument/result counts and types, callable source and the boolean selector are validated. Unknown/nil callbacks and unsupported variadic plans fail explicitly. These are synchronous rules; they do not model goroutine scheduling or deferred invocation.

MaxIterations defaults to 32 and participates in the source fingerprint. Repetitions also share MaxCalls, MaxPaths, MaxDepth and context cancellation. A stable abstract state can be summarized when it has a legitimate interruption or normal-return exit. The comparison includes reachable captures, pointer cells, request conditions, current/committed headers, response status and body framing. It excludes unreachable temporaries from returned frames. A stable always-continue callback does not acquire a fabricated normal return. Changing states beyond the budget remain diagnostics.

The independent SDK fixture validates native itemSchema with hand-written expected items, callback termination and capture changes, commit ordering, interruption and stable repetition. The separate Gin adapter declares Stream's disconnection/step/Flush behavior through this API.

Complete arbitrary heap/map/slice aliasing, receiver/promotion/generic combinations, async/defer behavior, ordinary loop summaries and the full identity/provenance matrix remain required work. The tested source-level closure cells do not establish unique runtime identities for route-registration closures or receiver instances; those runtime evidence boundaries remain separate.
