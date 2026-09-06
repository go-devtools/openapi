const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const path = require("node:path");

// Test only this project's display extension, using a lightweight React substitute to check props and refs.
function wrapper(name = "JSONSchema202012") {
  const context = { window: {} };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, "display-names.js"), "utf8"), context);
  const React = {
    forwardRef: (render) => render,
    createElement: (component, props, ...children) => ({ component, props, children }),
  };
  return context.window.OpenAPIDisplayNames().wrapComponents[name]("original", { React });
}

// Keep input and output projections linked to their components while displaying the shared business type name.
test("Different identities share one display name", () => {
  const render = wrapper();
  for (const key of ["User_request_123", "User_response_456"]) {
    const schema = { title: "User", $$ref: "#/components/schemas/" + key };
    const props = { schema, name: key, identifier: key };
    const ref = {};
    const result = render(props, ref);
    assert.equal(result.props.name, "User");
    assert.equal(result.props.schema, schema);
    assert.equal(result.props.ref, ref);
    assert.equal(result.props.identifier, key);
    assert.equal(props.name, key);
  }
});

// Preserve field names and external schemas without display titles instead of applying suffix rules.
test("Field labels and unknown titles retain their original values", () => {
  const render = wrapper();
  const schema = { title: "User", $$ref: "#/components/schemas/User_123" };
  assert.equal(render({ name: "Owner", schema }).props.name, "Owner");
  assert.equal(render({ name: "User_123", schema: { $$ref: schema.$$ref } }).props.name, "User_123");
  assert.equal(render({ name: "Field_123", schema: { title: "Other" } }).props.name, "Field_123");
});

// Display false, zero, null, and complex values individually without numbered schema arrays.
test("Zero values and complex examples render as safe text", () => {
  const render = wrapper("JSONSchema202012KeywordExamples");
  const examples = [0, false, null, "<script>alert(1)</script>", { a: [1, 2] }];
  const tree = render({ schema: { examples } });
  const items = tree.children[1].children[0];
  assert.equal(items[0].children[0].children[0], "0");
  assert.equal(items[1].children[0].children[0], "false");
  assert.equal(items[2].children[0].children[0], "null");
  assert.equal(items[3].children[0].component, "code");
  assert.equal(items[3].children[0].children[0], JSON.stringify(examples[3]));
  assert.equal(items[4].children[0].component, "pre");
  assert.equal(items[4].children[0].children[0], JSON.stringify(examples[4], null, 2));
  assert.equal(render({ schema: { examples: [] } }), null);
});

// Show enum choices directly without turning numbers, booleans, or null into string contracts.
test("Enums list all allowed values directly", () => {
  const render = wrapper("JSONSchema202012KeywordEnum");
  const values = ["admin", "editor", 0, false, null, { kind: "fixed" }];
  const tree = render({ schema: { enum: values } });
  assert.equal(tree.props["aria-label"], "Allowed values");
  assert.equal(tree.children[0].children[0], "Enum");
  const items = tree.children[1].children[0];
  assert.deepEqual(Array.from(items, item => item.children[0].children[0]), values.map(value => JSON.stringify(value, null, value !== null && typeof value === "object" ? 2 : undefined)));
  assert.equal(render({ schema: {} }), null);
});

// Align descriptions with enum indices and render them as text without allowing HTML insertion.
test("Enums display aligned descriptions and preserve values", () => {
  const render = wrapper("JSONSchema202012KeywordEnum");
  const schema = { enum: ["admin", "editor", 0, false], "x-enum-descriptions": ["Administrator", "Editor", "Pending", "<img src=x onerror=alert(1)>"] };
  const tree = render({ schema });
  const items = tree.children[1].children[0];
  assert.deepEqual(Array.from(items, item => item.children[0].children[0]), ['"admin" - Administrator', '"editor" - Editor', '0 - Pending', 'false - <img src=x onerror=alert(1)>']);
  assert.deepEqual(schema.enum, ["admin", "editor", 0, false]);
  assert.equal(items[3].children[0].props, null);
});

// Avoid misleading labels when descriptions are missing or mismatched.
test("Enums without descriptions display only values", () => {
  const render = wrapper("JSONSchema202012KeywordEnum");
  for (const descriptions of [undefined, ["Administrator"], ["", null]]) {
    const tree = render({ schema: { enum: ["admin", "editor"], "x-enum-descriptions": descriptions } });
    assert.deepEqual(Array.from(tree.children[1].children[0], item => item.children[0].children[0]), ['"admin"', '"editor"']);
  }
});
