const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const path = require("node:path");

// Test only this project's display extension, using a lightweight React substitute to check props and refs.
// 只运行本项目展示扩展，用轻量 React 替身检查属性与引用传递。
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
// 输入输出投影仍分别引用原组件，但渲染只展示共同业务类型名。
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
// 字段名与无展示标题的外部 Schema 不被后缀规则误改。
test("Field labels and unknown titles retain their original values", () => {
  const render = wrapper();
  const schema = { title: "User", $$ref: "#/components/schemas/User_123" };
  assert.equal(render({ name: "Owner", schema }).props.name, "Owner");
  assert.equal(render({ name: "User_123", schema: { $$ref: schema.$$ref } }).props.name, "User_123");
  assert.equal(render({ name: "Field_123", schema: { title: "Other" } }).props.name, "Field_123");
});

// Display false, zero, null, and complex values individually without numbered schema arrays.
// false、零、null 和复杂值逐项显示，不制造带编号的 Schema 数组。
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
// 枚举直接展示可选值，数字、布尔和空值不会被转成字符串契约。
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
// 说明与枚举索引一一对应，并以文本呈现，不能通过说明插入 HTML。
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
// 缺失或不匹配的说明不制造误导性的标签。
test("Enums without descriptions display only values", () => {
  const render = wrapper("JSONSchema202012KeywordEnum");
  for (const descriptions of [undefined, ["Administrator"], ["", null]]) {
    const tree = render({ schema: { enum: ["admin", "editor"], "x-enum-descriptions": descriptions } });
    assert.deepEqual(Array.from(tree.children[1].children[0], item => item.children[0].children[0]), ['"admin"', '"editor"']);
  }
});
