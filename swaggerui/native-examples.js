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

  // Parameter controls accept logical values; applying serialized text would encode it a second time.
  function withParameterExamples(parameter) {
    if (!parameter || !parameter.has("examples")) return parameter;
    return parameter.update("examples", examples => examples.map(example =>
      example.has("dataValue") ? example.set("value", example.get("dataValue")) : example));
  }

  // Match the values accepted by upstream scalar, array, and object form controls.
  function fieldValue(field, data, name, fn) {
    let value = data.has(name) ? plainValue(data.get(name)) : fn.getSampleSchema(field, false, { includeWriteOnly: true });
    if (typeof value === "boolean" || typeof value === "number") value = String(value);
    else if (value !== null && typeof value === "object" && !Array.isArray(value)) value = JSON.stringify(value, null, 2);
    return value;
  }

  // Recognize unchanged form defaults so switching media does not retain a field map as a JSON body.
  function unchangedForm(system, path, method) {
    if (!isNative(system)) return undefined;
    const type = system.oas3Selectors.requestContentType(path, method);
    if (type !== "application/x-www-form-urlencoded" && type !== "multipart/form-data") return undefined;
    const media = system.specSelectors.specResolvedSubtree(["paths", path, method, "requestBody"])?.getIn(["content", type]);
    const examples = media?.get("examples");
    const key = system.oas3Selectors.activeExamplesMember(path, method, "requestBody", "requestBody");
    const data = examples?.get(examples.has(key) ? key : examples.keySeq().first())?.get("dataValue");
    const fields = media?.getIn(["schema", "properties"]);
    const current = system.oas3Selectors.requestBodyValue(path, method);
    if (!data || typeof data.has !== "function" || !fields || !current?.getIn) return undefined;
    return fields.every((field, name) => {
      const value = current.getIn([name, "value"]);
      if (field.get("readOnly") || system.fn.isFileUploadIntended(field)) return value === undefined;
      return JSON.stringify(plainValue(value)) === JSON.stringify(fieldValue(field, data, name, system.fn));
    });
  }

  // Share the same native default between the editor and upstream edit-retention decisions.
  function defaultValue(system, path, method) {
    if (!isNative(system)) return undefined;
    const mediaType = system.oas3Selectors.requestContentType(path, method);
    const owner = system.specSelectors.specResolvedSubtree(["paths", path, method, "requestBody"]);
    const media = owner?.getIn(["content", mediaType]);
    const examples = media?.get("examples");
    const key = system.oas3Selectors.activeExamplesMember(path, method, "requestBody", "requestBody");
    const example = examples?.get(examples.has(key) ? key : examples.keySeq().first());
    const value = exampleValue(example, mediaType || "");
    if (value === undefined) return undefined;
    if (value instanceof WireExample) return value.text;
    const sampled = system.fn.getSampleSchema(media.get("schema"), mediaType, { includeWriteOnly: true }, value);
    return typeof sampled === "string" ? sampled : JSON.stringify(plainValue(sampled), null, 2);
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
    statePlugins: {
      oas3: {
        wrapSelectors: {
          // Reset and media selection must compare against the example that the reader actually sees.
          selectDefaultRequestBodyValue: (original, system) => (state, path, method) =>
            defaultValue(system, path, method) ?? original(state, path, method),
          hasUserEditedBody: (original, system) => (state, path, method) => {
            const form = unchangedForm(system, path, method);
            if (form !== undefined) return !form;
            const expected = defaultValue(system, path, method);
            const current = system.oas3Selectors.requestBodyValue(path, method);
            if (expected === undefined || (current && typeof current === "object")) return original(state, path, method);
            return current !== null && current !== undefined && current !== expected;
          },
          shouldRetainRequestBodyValue: (original, system) => (state, path, method) => {
            if (unchangedForm(system, path, method)) return false;
            const expected = defaultValue(system, path, method);
            return expected !== undefined && system.oas3Selectors.requestBodyValue(path, method) === expected
              ? false : original(state, path, method);
          }
        }
      }
    },
    wrapComponents: {
      response: wrapOwner("response"),
      // Retain upstream header descriptions and types while exposing native Example fields separately.
      headers: function (Original, system) {
        return function NativeHeaderExamples(props) {
          const h = system.React.createElement;
          const panels = [];
          if (isNative(system)) props.headers?.forEach((header, name) => header.get("examples")?.forEach((example, key) => {
            if (!example.has("dataValue") && !example.has("serializedValue") && !example.has("externalValue")) return;
            panels.push(h("details", { key: name + ":" + key }, h("summary", null, name + " — " + key),
              example.has("summary") ? h("p", null, example.get("summary")) : null,
              example.has("description") ? h(props.getComponent("Markdown", true), { source: example.get("description") }) : null,
              example.has("dataValue") ? h("div", null, h("strong", null, "Data value"),
                h("pre", { "aria-label": "Header data value" }, JSON.stringify(plainValue(example.get("dataValue")), null, 2))) : null,
              example.has("serializedValue") ? h("div", null, h("strong", null, "Serialized value"),
                h("pre", { "aria-label": "Header serialized value" }, example.get("serializedValue"))) : null,
              example.has("externalValue") ? h("p", null, "External example (not fetched): ", h("code", null, example.get("externalValue"))) : null));
          }));
          return h(system.React.Fragment, null, h(Original, props), panels.length ?
            h("section", { className: "openapi-header-examples", "aria-label": "Response header examples" }, h("h4", null, "Header examples"), panels) : null);
        };
      },
      // Feed parameter examples through both the visible row and its identity-based default lookup.
      parameterRow: function (Original, system) {
        return function NativeParameterExamples(props) {
          if (!isNative(system)) return system.React.createElement(Original, props);
          const selectors = Object.assign({}, props.specSelectors, {
            parameterWithMetaByIdentity: (...args) => withParameterExamples(props.specSelectors.parameterWithMetaByIdentity(...args))
          });
          return system.React.createElement(Original, Object.assign({}, props, {
            param: withParameterExamples(props.param), specSelectors: selectors
          }));
        };
      },
      RequestBody: function (Original, system) {
        const Wrapped = wrapOwner("requestBody")(Original, system);
        return function NativeFormExamples(props) {
          const h = system.React.createElement;
          const mediaType = props.contentType || props.requestBody?.get("content")?.keySeq().first();
          const media = props.requestBody?.getIn(["content", mediaType]);
          const available = media?.get("examples");
          const activeKey = available?.has(props.activeExamplesKey) ? props.activeExamplesKey : available?.keySeq().first();
          const properties = media?.getIn(["schema", "properties"]);
          const form = isNative(system) && (mediaType === "application/x-www-form-urlencoded" || mediaType === "multipart/form-data");
          const examples = form && properties ? media.get("examples")?.filter(example => {
            const value = example.get("dataValue");
            return value && typeof value.has === "function" && !Array.isArray(plainValue(value));
          }) : null;
          const selected = examples?.has(props.activeExamplesKey) ? props.activeExamplesKey : examples?.keySeq().first();
          const example = examples?.get(selected);
          const data = example?.get("dataValue");
          const projected = data ? props.requestBody.updateIn(["content", mediaType, "schema", "properties"], fields =>
            fields.map((field, name) => data.has(name) ? field.set("example", data.get(name)) : field)) : props.requestBody;
          // Apply a selection once; subsequent typing or response changes must not overwrite the form.
          system.React.useEffect(() => {
            if (!data || !props.isExecute) return;
            if (selected !== props.activeExamplesKey) props.updateActiveExamplesKey(selected);
            properties.forEach((field, name) => {
              if (field.get("readOnly") || props.fn.isFileUploadIntended(field)) return;
              props.onChange(fieldValue(field, data, name, props.fn), [name]);
            });
          }, [mediaType, selected, data, props.isExecute]);
          return h(system.React.Fragment, null,
            examples?.size ? h("label", { className: "openapi-form-examples" }, "Examples: ",
              h("select", { "aria-label": "Form example", value: selected, onChange: event => props.updateActiveExamplesKey(event.target.value) },
                examples.keySeq().toArray().map(key => h("option", { key: key, value: key }, key)))) : null,
            h(Wrapped, Object.assign({}, props, { key: mediaType, requestBody: projected, activeExamplesKey: activeKey })),
            example?.has("serializedValue") ? h("section", { className: "example__section", "aria-label": "Serialized form example" },
              h("div", { className: "example__section-header" }, "Serialized example"),
              h("p", null, "Reference wire text. Editable fields use the logical data and the selected form encoding."),
              h(props.getComponent("HighlightCode", true), { language: "text" }, example.get("serializedValue"))) : null,
            example ? h(props.getComponent("Example"), { example: example, getComponent: props.getComponent, getConfigs: props.getConfigs }) : null);
        };
      },
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
