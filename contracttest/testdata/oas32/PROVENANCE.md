# Official validation resources

These original test bytes come from the OpenAPI Initiative and retain the upstream Apache 2.0 license. Tests read them directly; the production root package and Swagger UI do not embed them.

| File | Pinned source | SHA-256 |
| --- | --- | --- |
| schema.json | https://spec.openapis.org/oas/3.2/schema/2025-11-23 | 7d48f01f37eeae4799041b371ad5f533f9f533fd2b0caa1011a8ba27c5b48b70 |
| schema-base.json | https://spec.openapis.org/oas/3.2/schema-base/2025-11-23 | 423daa88e2285fa343856c08502fe63fd8aa3674cd5b4ef88746ba6f82647af3 |
| dialect.json | https://spec.openapis.org/oas/3.2/dialect/2025-09-17 | 4e2c989f3d1e6489d41bc1ca4ade11743278c612b82fba9144c6116c79f1c273 |
| meta.json | https://spec.openapis.org/oas/3.2/meta/2025-09-17 | a1959c0aa1f9a7ce58f2b75699be5bc5187c8a25901fe04a227f1d9419ba4e9c |

The schema-base resource requires the explicit fixed dialect listed above. The full standard fixture is checked against that dialect. Product defaults follow OpenAPI 3.2: `https://spec.openapis.org/oas/3.1/dialect/base`. The test resource does not redefine this default.

License source: https://raw.githubusercontent.com/OAI/OpenAPI-Specification/3.2.0/LICENSE . Tests use an independent JSON Schema 2020-12 engine with network and file loaders disabled. Structural checking does not replace cross-field semantics, application behavior, or reference-target validation.
