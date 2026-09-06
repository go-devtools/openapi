# Finite request conditions

`RequestCondition` describes exact method and media-type inclusion/exclusion sets. Empty inclusion sets leave that dimension unrestricted. An empty media string represents an absent Content-Type; media parameters and wildcards are not accepted as selector values. The frontend supplies selector semantics, so the core does not guess how a framework normalizes headers.

`RequestCondition.Intersect` returns a detached normalized condition, a reachability boolean, and an error. Mutually exclusive conditions are unreachable; they never turn into an unrestricted condition. Invalid values and excessive set sizes are rejected. `CallOutcome.When` applies the condition to a call's correlated results and effects. The analyzer intersects it with the current path, including through helper calls and subsequent binders.

```go
// Declare call effects for a finite request domain while the frontend defines wire rules.
outcome := compiler.CallOutcome{
    When: openapi.RequestCondition{
        ExceptMethods: []string{"GET"},
        MediaTypes: []string{"application/json"},
    },
    Results: []compiler.Value{{Nil: true}},
    Effects: requestEffects,
}
```

The compiler groups complete paths with identical conditions into `Template.Variants`. Each `OperationVariant` stores a projected operation, applicable diagnostics, and source facts. It contains no AST, type objects, callbacks, pointers to business values, or framework objects. Such bundles require the `request-conditions-v1` capability. Existing unconditional bundles remain supported; absent capability declarations and unknown future capabilities fail explicitly.

At startup, `Build` selects variants using `Route.Method` and `Route.RequestMediaTypes`. Method-only tables do not require media configuration. A method whose applicable variants depend on media needs an explicit finite media list; a missing list, an uncovered selection, or a selected unresolved codec prevents document construction. Diagnostics from other methods/media do not invalidate the selected operation. The runtime never re-runs source analysis.

Multiple selected media are supported when their contracts can be combined without losing meaning. Content maps retain distinct media keys; overlapping response schemas use `anyOf`, preserving composition siblings and unconstrained alternatives. Different parameter contracts or body-required values are diagnosed instead of silently merging incompatible location/presence requirements. Metadata conflicts and incompatible references are likewise rejected. Returned snapshots, documents, and reports remain detached from caller-owned data.

Selected facts and diagnostics carry `Source.When`. Explicit route media configuration contributes a `declared` provenance fact. Conditions describe applicability; they do not prove that a server rejects every undeclared input. `Effect.AlternativeLocations` lets a frontend indicate that logical input may come from more than one location. A cross-location field-required declaration is diagnosed when it cannot be expressed as local parameter/body requirements without changing its meaning.

Tests cover serialized capability gating, method/media selection, unknown and excluded codecs, multi-media schema union, condition intersection and ownership, request-body presence ambiguity, two successive calls with incompatible conditions, and independent external SDK consumption. Build inputs and freshness are described in [build inputs](build-inputs.md). These condition tests do not establish arbitrary alias/helper semantics or every framework codec combination.
