// Adapt native Example values only at the component boundary, preserving the source document.
window.OpenAPINativeExamples = function () {
  // Keep explicit wire text separate from legacy examples that still require schema sampling.
  class WireExample {
    // Keep empty strings and explicit primitive values distinguishable from missing examples.
    constructor(text) { this.text = text; }
  }

  // Limit the adapter to the native 3.2 Example contract.
  function isNative(system) {
    return /^3\.2\.\d+$/.test(system.specSelectors.specJson().get("openapi", ""));
  }

  // Convert immutable example data without treating ordinary business objects as Example Objects.
  function plainValue(value) {
    return value && typeof value.toJS === "function" ? value.toJS() : value;
  }

  // Select an explicit serialization before deriving JSON from schema-ready data.
  function exampleValue(example, mediaType) {
    if (!example || typeof example.has !== "function") return undefined;
    if (example.has("serializedValue")) return new WireExample(example.get("serializedValue"));
    if (!example.has("dataValue")) return undefined;
    const value = plainValue(example.get("dataValue"));
    const media = mediaType.split(";", 1)[0].trim().toLowerCase();
    if (media === "application/json" || media.endsWith("+json")) {
      return new WireExample(JSON.stringify(value, null, 2));
    }
    if (media === "text/plain" && typeof value === "string") return new WireExample(value);
    return value;
  }

  // Copy only content examples; never traverse or rewrite arbitrary data or schema properties.
  function withExamples(owner) {
    if (!owner || !owner.has("content")) return owner;
    return owner.update("content", function (content) {
      return content.map(function (media, mediaType) {
        if (!media || !media.has("examples")) return media;
        return media.update("examples", function (examples) {
          return examples.map(function (example) {
            const value = exampleValue(example, mediaType);
            return value === undefined ? example : example.set("value", value);
          });
        });
      });
    });
  }

  // Reuse upstream selection, editor, and response state while bypassing sampling for exact wire text.
  function wrapOwner(property) {
    return function (Original, system) {
      return function NativeExamples(props) {
        if (!isNative(system)) return system.React.createElement(Original, props);
        const fn = Object.assign({}, props.fn, {
          getSampleSchema: function (schema, media, options, example) {
            if (example instanceof WireExample) return example.text;
            return props.fn.getSampleSchema(schema, media, options, example);
          }
        });
        return system.React.createElement(Original, Object.assign({}, props, {
          [property]: withExamples(props[property]), fn: fn
        }));
      };
    };
  }

  return {
    wrapComponents: {
      response: wrapOwner("response"),
      RequestBody: wrapOwner("requestBody"),
      // Keep paired schema-ready data visible below its wire example, preserving upstream metadata.
      Example: function (Original, system) {
        return function NativeExampleDetails(props) {
          const h = system.React.createElement;
          const example = props.example;
          const original = h(Original, props);
          if (!isNative(system) || !example || !example.has("dataValue") ||
              (!example.has("serializedValue") && !example.has("externalValue"))) return original;
          const HighlightCode = props.getComponent("HighlightCode", true);
          return h(system.React.Fragment, null, original,
            h("section", { className: "example__section", "aria-label": "Data value" },
              h("div", { className: "example__section-header" }, "Data value"),
              h(HighlightCode, { language: "json" }, JSON.stringify(plainValue(example.get("dataValue")), null, 2))));
        };
      }
    }
  };
};
