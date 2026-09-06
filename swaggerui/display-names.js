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
  return {
    wrapComponents: {
      // Render Schema.examples as example values rather than an indexed schema array.
      JSONSchema202012KeywordExamples: function (_Original, system) {
        return function SchemaExamples(props) {
          const examples = props.schema && props.schema.examples;
          return valueList(system, examples, examples && examples.length === 1 ? "Example" : "Examples", "Examples");
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
