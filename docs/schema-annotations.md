# Schema checks and source annotations

`openapi.Check` and `CheckWithOptions` validate standard keyword values, object contexts, and references in raw documents. `compiler.Project.Schema` additionally checks source declarations against inferred wire types, fixed lengths, and numeric bounds.

## Raw Schema validity

JSON Schema permits constraints that do not apply to an instance type and schemas that no instance can satisfy. Both examples are valid schemas:

```json
{"type":"string","minimum":1}
```

```json
{"type":"string","minLength":10,"maxLength":2}
```

`minimum` does not constrain strings. The second Schema accepts no string. The raw checker does not classify either case as a syntax error. Empty and duplicate enum values are not rejected merely for violating advisory recommendations.

Standard keyword values must be valid: type names must be recognized; a type array must be nonempty and unique; lengths must be nonnegative integers; multipleOf must be positive. Composition and prefixItems take nonempty Schema arrays, while items takes one Schema. Keys in properties and `$defs` remain property or definition names, including names beginning with `x-`.

Numbers are examined as decimal text without floating-point conversion or exponent-sized integer expansion. For example, `100.00e-2` is integral and `100.00e-3` is not. Large exponents remain bounded by the input budget.

The checker does not implement full ECMA-262 pattern syntax validation. Format assertions, instance validation, and complete custom-dialect semantics require an independent validator. Keyword validation alone does not establish every aspect of standards conformance.

## Source declarations and inferred facts

A source string field declaring `minimum=1` produces `openapi.comment.type`; minLength greater than maxLength produces `openapi.comment.range`. These are source-contract diagnostics and do not redefine raw JSON Schema validity.

Checks run after the component graph is available, so a field referencing a named type still inherits its actual type and bounds.

| Code | Meaning |
| --- | --- |
| `openapi.comment.type` | Constraint does not apply to the actual wire type. |
| `openapi.comment.range` | Declared bounds do not intersect, including field references and component bounds. |
| `openapi.comment.derived` | Declaration overrides a fixed array length or relaxes a type-derived numeric bound. |
| `openapi.comment.value` | Invalid keyword value, such as required=5, minLength=null, or multipleOf=0. |
| `openapi.comment.budget` | Exact bound comparison exceeds the compilation budget. |

The required, nullable, nonnull, and ignore flags must be booleans. In Schema comments, required applies only to fields. Function-level request declarations separately accept body-level required; see [request and response declarations](request-response-declarations.md). nullable is idempotent for a direct type union already containing null. Declarations cannot hide transmitted fields, fabricate types, or mark actually emitted fields as write-only.

Compile-time bound comparison allows at most 4,096 text digits and decimal exponents with absolute value at most 4,096. Exceeding these limits returns a budget error without large integer expansion. This is an implementation limit for source comparisons, not a JSON Schema limit on raw numbers.

Range checks cover standard projections, named components, and simple null unions. General composition satisfiability, complete example encodability, nullable/nonnull rewriting of reference unions, and every source-location combination are outside this coverage. A declaration does not prove that the server enforces the constraint.

## Tests and references

The 52 cases in `testdata/golden/schema-keywords.json` run against both the project checker and a fixed official OpenAPI 3.2 meta-schema. Source tests cover actual Go types and comments, named references, fixed arrays, unsigned values, precise integers, and extreme exponents. Numeric classification fuzzing uses an independently calculated bounded rational value.

The keyword rules come from [JSON Schema 2020-12 Validation](https://json-schema.org/draft/2020-12/json-schema-validation) and [Core](https://json-schema.org/draft/2020-12/json-schema-core). Fixed upstream resource provenance and checksums are in [PROVENANCE.md](../contracttest/testdata/oas32/PROVENANCE.md).
