// Display schema titles while preserving internal references and component state identities.
window.OpenAPIDisplayNames = function () {
  // Render JSON values only as React text nodes, preserving indentation and separate scrolling for complex values.
  function valueList(system, values, label, ariaLabel, descriptions) {
    if (!Array.isArray(values) || values.length === 0) return null;
    const h = system.React.createElement;
    return h("section", { className: "openapi-field-examples", "aria-label": ariaLabel },
      h("span", { className: "openapi-field-examples__label" }, label),
      h("ul", { className: "openapi-field-examples__values" }, values.map(function (value, index) {
        const structured = value !== null && typeof value === "object";
        const encoded = JSON.stringify(value, null, structured ? 2 : undefined);
        const hasDescriptions = Array.isArray(descriptions) && descriptions.length === values.length;
        const description = hasDescriptions && typeof descriptions[index] === "string" ? descriptions[index].trim() : "";
        return h("li", { key: index }, h(structured ? "pre" : "code", null, encoded + (description ? " - " + description : "")));
      })));
  }
  // Resolve only declared local component titles, leaving external or unknown references as literal text.
  function mappingLabel(system, reference) {
    if (typeof reference !== "string") return reference;
    let key = reference;
    const marker = "#/components/schemas/";
    if (reference.startsWith(marker)) {
      key = reference.slice(marker.length);
      try { key = decodeURIComponent(key); } catch (_) { return reference; }
      if (key.includes("/")) return reference;
      key = key.replace(/~1/g, "/").replace(/~0/g, "~");
    }
    const source = system.specSelectors.specJson();
    const title = source && source.getIn && source.getIn(["components", "schemas", key, "title"]);
    return typeof title === "string" && title.length > 0 ? title : reference;
  }
  return {
    wrapComponents: {
      // Retain upstream sibling metadata while replacing only the indexed examples display.
      JSONSchema202012KeywordExamples: function (Original, system) {
        return function SchemaExamples(props) {
          const schema = props.schema;
          const examples = schema && schema.examples;
          const projected = schema && typeof schema === "object" ? Object.assign({}, schema) : schema;
          if (projected && typeof projected === "object") delete projected.examples;
          return system.React.createElement(system.React.Fragment, null,
            valueList(system, examples, examples && examples.length === 1 ? "Example" : "Examples", "Examples"),
            system.React.createElement(Original, Object.assign({}, props, { schema: projected })));
        };
      },
      // Show title-based mapping labels without changing references used by the document or validator.
      JSONSchema202012KeywordDiscriminator: function (Original, system) {
        return function SchemaDiscriminator(props) {
          const schema = props.schema;
          const discriminator = schema && schema.discriminator;
          const h = system.React.createElement;
          if (!discriminator || typeof discriminator !== "object") return h(Original, props);
          const projected = Object.assign({}, discriminator);
          if (discriminator.mapping && typeof discriminator.mapping === "object") {
            projected.mapping = Object.fromEntries(Object.entries(discriminator.mapping).map(function (entry) {
              return [entry[0], mappingLabel(system, entry[1])];
            }));
          }
          const native = system.specSelectors.isOAS32 && system.specSelectors.isOAS32();
          const hasDefault = native && Object.prototype.hasOwnProperty.call(discriminator, "defaultMapping");
          return h(system.React.Fragment, null,
            h(Original, Object.assign({}, props, { schema: Object.assign({}, schema, { discriminator: projected }) })),
            discriminator.propertyName === "" ? h("div", { className: "openapi-schema-metadata" },
              h("span", null, "Discriminator property "), h("code", null, '\"\"')) : null,
            hasDefault ? h("section", { className: "openapi-schema-metadata", "aria-label": "Default mapping" },
              h("strong", null, "Default mapping "), h("code", null, mappingLabel(system, discriminator.defaultMapping)),
              h("p", null, "Expected schema for a missing or unmatched discriminator value. Schema validation still applies.")) : null);
        };
      },
      // Add the native XML node annotation alongside the existing name, namespace and extension display.
      JSONSchema202012KeywordXml: function (Original, system) {
        return function SchemaXml(props) {
          const xml = props.schema && props.schema.xml;
          const native = system.specSelectors.isOAS32 && system.specSelectors.isOAS32();
          const h = system.React.createElement;
          return h(system.React.Fragment, null, h(Original, props),
            native && xml && Object.prototype.hasOwnProperty.call(xml, "nodeType") ?
              h("div", { className: "openapi-schema-metadata" }, h("strong", null, "XML nodeType "),
                h("code", { "aria-label": "XML node type" }, xml.nodeType)) : null);
        };
      },
      // Display enum values directly while preserving their original JSON types.
      JSONSchema202012KeywordEnum: function (_Original, system) {
        return function SchemaEnum(props) {
          return valueList(system, props.schema && props.schema.enum, "Enum", "Allowed values", props.schema && props.schema["x-enum-descriptions"]);
        };
      },
      // Forward upstream refs so model expansion and scrolling retain the original component lifecycle.
      JSONSchema202012: function (Original, system) {
        return system.React.forwardRef(function DisplaySchemaName(props, ref) {
          const schema = props.schema;
          let displayName = props.name;
          const reference = schema && (schema.$$ref || schema.$ref);
          if (schema && typeof schema.title === "string" && typeof reference === "string") {
            const marker = "#/components/schemas/";
            const position = reference.indexOf(marker);
            if (position >= 0) {
              // Replace only titles originating from this reference, without guessing suffixes or changing business field names.
              const encoded = reference.slice(position + marker.length);
              let key = encoded;
              try { key = decodeURIComponent(encoded); } catch (_) { /* Preserve the original value for invalid URIs. */ }
              key = key.replace(/~1/g, "/").replace(/~0/g, "~");
              if (props.name === key) displayName = schema.title;
            }
          }
          return system.React.createElement(Original, Object.assign({}, props, { name: displayName, ref: ref }));
        });
      }
    }
  };
};
