// Preserve stream item semantics and prevent known lossy whole-query submissions in the pinned renderer.
window.OpenAPINativeWire = function () {
  // Convert only the requested immutable value, leaving the browser's source document untouched.
  function plain(value) { return value && typeof value.toJS === 'function' ? value.toJS() : value; }

  // Resolve bounded local JSON Pointer aliases without fetching resources or following cycles.
  function resolve(value, document) {
    const seen = new Set();
    while (value && typeof value === 'object' && typeof value.$ref === 'string' && !seen.has(value)) {
      if (seen.size >= 64 || !value.$ref.startsWith('#/') || value.$ref.length > 16384) return value;
      seen.add(value);
      let parts;
      try { parts = decodeURIComponent(value.$ref.slice(2)).split('/'); } catch (_) { return value; }
      let target = document;
      for (const part of parts) {
        const key = part.replace(/~1/g, '/').replace(/~0/g, '~');
        if (!target || typeof target !== 'object' || !Object.hasOwn(target, key)) return value;
        target = target[key];
      }
      value = target;
    }
    return value;
  }

  // Detect this known serialization gap from the actual operation, including inherited and referenced parameters.
  function hasWholeQuery(document, path, method, operation) {
    if (!document || !/^3\.2\.\d+$/.test(document.openapi)) return false;
    const item = resolve(document.paths && document.paths[path], document);
    const declared = resolve(item && item[method], document);
    return [item, declared, plain(operation)].some(owner => owner && Array.isArray(owner.parameters) &&
      owner.parameters.some(parameter => resolve(parameter, document)?.in === 'querystring'));
  }

  // Inspect only XML schema positions, bounding reference traversal and recursive schemas.
  function xmlNodes(schema, document) {
    const found = [];
    const seen = new Set();
    let remaining = 10000;
    // Keep annotation paths distinct from arbitrary example data and business property names.
    function visit(value, path, depth) {
      value = resolve(value, document);
      if (!value || typeof value !== 'object' || seen.has(value) || --remaining < 0 || depth > 64) return;
      seen.add(value);
      const node = value.xml?.nodeType;
      if (node && found.length < 100) found.push({ path: path, nodeType: node });
      for (const key of ['properties', 'patternProperties', 'dependentSchemas']) {
        const entries = value[key];
        if (entries && typeof entries === 'object') for (const name of Object.keys(entries)) {
          if (remaining < 0) break;
          visit(entries[name], path + '/' + key + '/' + name.replace(/~/g, '~0').replace(/\//g, '~1'), depth + 1);
        }
      }
      for (const key of ['items', 'contains', 'additionalProperties', 'unevaluatedProperties', 'not', 'if', 'then', 'else'])
        visit(value[key], path + '/' + key, depth + 1);
      for (const key of ['allOf', 'anyOf', 'oneOf', 'prefixItems']) if (Array.isArray(value[key])) {
        for (let index = 0; index < value[key].length && remaining >= 0; index++) visit(value[key][index], path + '/' + key + '/' + index, depth + 1);
      }
    }
    visit(schema, '#/schema', 0);
    return found;
  }

  // Return measured renderer limits for one selected media contract, leaving alternatives available.
  function bodyLimitation(document, path, method, contentType, exampleKey) {
    if (!document || !/^3\.2\.\d+$/.test(document.openapi) || !contentType) return null;
    const item = resolve(document.paths?.[path], document);
    const body = resolve(item?.[method]?.requestBody, document);
    const media = resolve(body?.content?.[contentType], document);
    if (!media) return null;
    const type = contentType.split(';', 1)[0].trim().toLowerCase();
    if (type.startsWith('multipart/') && (type !== 'multipart/form-data' || Object.hasOwn(media, 'prefixEncoding') || Object.hasOwn(media, 'itemEncoding'))) {
      return { code: 'openapi.ui.multipart', message: 'This viewer does not encode positional multipart bodies (prefixEncoding / itemEncoding) or multipart media other than multipart/form-data.',
        fix: 'Use a compatible multipart client or select another supported request media type.',
        metadata: { mediaType: contentType, prefixEncoding: media.prefixEncoding, itemEncoding: media.itemEncoding } };
    }
    if (type === 'application/xml' || type === 'text/xml' || type.endsWith('+xml')) {
      const nodes = xmlNodes(media.schema, document);
      const examples = media.examples;
      const key = examples && Object.hasOwn(examples, exampleKey) ? exampleKey : Object.keys(examples || {})[0];
      const example = resolve(examples?.[key], document);
      if (nodes.length && typeof example?.serializedValue !== 'string') {
        return { code: 'openapi.ui.xmlNodeType', message: 'The XML sampler does not implement native nodeType annotations. Its generated XML is not a reliable wire example.',
          fix: 'Provide a serializedValue XML example or use a client implementing the declared XML node types.', metadata: nodes };
      }
    }
    return null;
  }

  let previous;
  let document;

  // Cache the plain document once per immutable source update rather than copying it for every operation.
  function currentDocument(system) {
    const source = system.specSelectors.specJson();
    if (source !== previous) { previous = source; document = source.toJS(); }
    return document;
  }

  // Read the current native document only when a component or request needs this compatibility decision.
  function blocked(system, path, method, operation) {
    return hasWholeQuery(currentDocument(system), path, method, operation);
  }

  // Consult the currently selected media for both the control and the actual execution action.
  function currentBodyLimitation(system, path, method, contentType) {
    const selectors = system.oas3Selectors;
    return bodyLimitation(currentDocument(system), path, method,
      contentType || selectors?.requestContentType(path, method),
      selectors?.activeExamplesMember(path, method, 'requestBody', 'requestBody'));
  }

  // Render one independently validated stream item, preserving boolean schemas and reference siblings.
  function itemPanel(system, props, owner, contentType, label, response) {
    if (!/^3\.2\.\d+$/.test(system.specSelectors.specJson().get('openapi', ''))) return null;
    const content = owner && owner.get('content');
    if (!content || typeof content.get !== 'function') return null;
    const mediaType = contentType || content.keySeq().first();
    const media = content.get(mediaType);
    if (!media || !media.has('itemSchema')) return null;
    const schema = media.get('itemSchema');
    const h = system.React.createElement;
    const Model = props.getComponent('Model', true);
    const visual = typeof schema === 'boolean'
      ? h('code', null, schema ? 'true — Any stream item is allowed.' : 'false — No stream item is valid.')
      : h(Model, { schema: schema, specPath: props.specPath.push('content', mediaType, 'itemSchema'), getComponent: props.getComponent, getConfigs: props.getConfigs });
    return h('section', { className: 'openapi-stream-item', 'aria-label': label },
      h('h4', null, 'Stream item schema'), h('code', null, mediaType),
      h('p', null, 'Applies independently to each item in this sequence.'),
      response ? h('p', { 'aria-label': 'Stream response display' }, 'Response bodies appear only after the response completes. This viewer does not display items incrementally; use a streaming client for long-lived responses.') : null, visual);
  }

  return {
    fn: { openapiHasWholeQuery: hasWholeQuery, openapiBodyLimitation: bodyLimitation },
    statePlugins: {
      spec: {
        wrapActions: {
          // Guard the actual action as well as the controls so an enabled method cannot silently omit the query value.
          executeRequest: function (original, system) {
            return function executeNativeRequest(request) {
              if (blocked(system, request.pathName, request.method, request.operation)) {
                return { type: 'openapi.ui.request.blocked', payload: { code: 'openapi.ui.querystring', severity: 'warning',
                  message: 'Whole-query serialization is unsupported by this renderer; no request was sent.',
                  fix: 'Use a client supporting the declared querystring media type and encoding.',
                  route: (/^(get|put|post|delete|options|head|patch|trace|query)$/.test(request.method) ? request.method.toUpperCase() : request.method) + ' ' + request.pathName } };
              }
              const limit = currentBodyLimitation(system, request.pathName, request.method, request.requestContentType);
              if (limit) return { type: 'openapi.ui.request.blocked', payload: { ...limit, severity: 'warning', route: request.method.toUpperCase() + ' ' + request.pathName } };
              return original(request);
            };
          }
        }
      }
    },
    wrapComponents: {
      // Explain the disabled action alongside the affected parameter, even far from the document header.
      parameterRow: function (Original, system) {
        return function NativeParameter(props) {
          const h = system.React.createElement;
          const original = h(Original, props);
          if (!/^3\.2\.\d+$/.test(system.specSelectors.specJson().get('openapi', '')) || props.rawParam?.get('in') !== 'querystring') return original;
          return h(system.React.Fragment, null, original,
            h('tr', null, h('td', { colSpan: 2 }, h('p', { className: 'openapi-wire-note' },
              'Submission is unavailable here: this viewer does not serialize whole-query parameters. Use a compatible client for the declared media type and encoding.'))));
        };
      },
      // Preserve read-only parameter details while removing the known lossy submission controls.
      operation: function (Original, system) {
        return function NativeOperation(props) {
          const operation = props.operation;
          const disabled = blocked(system, operation.get('path'), operation.get('method'), operation.get('op'));
          return system.React.createElement(Original, disabled ? Object.assign({}, props, {
            operation: operation.set('allowTryItOut', false).set('tryItOutEnabled', false)
          }) : props);
        };
      },
      // Re-evaluate the selected media without disabling working alternatives on the same operation.
      execute: function (Original, system) {
        return function NativeExecute(props) {
          const limit = currentBodyLimitation(system, props.path, props.method);
          return system.React.createElement(Original, limit ? Object.assign({}, props, { disabled: true }) : props);
        };
      },
      // Follow request media changes without changing the complete-body schema or wire editor.
      RequestBody: function (Original, system) {
        return function NativeRequestBody(props) {
          const h = system.React.createElement;
          const path = props.specPath.get(1);
          const method = props.specPath.get(2);
          const limit = currentBodyLimitation(system, path, method, props.contentType);
          const Highlight = props.getComponent('HighlightCode', true);
          return h(system.React.Fragment, null, h(Original, props),
            limit ? h('section', { className: 'openapi-wire-note', 'aria-label': 'Request encoding limitation' },
              h('strong', null, 'Submission unavailable for this media type'), h('p', null, limit.message), h('p', null, limit.fix),
              h('details', null, h('summary', null, 'Native encoding metadata'),
                h(Highlight, { language: 'json' }, JSON.stringify(limit.metadata, null, 2)))) : null,
            itemPanel(system, props, props.requestBody, props.contentType, 'Request stream item schema'));
        };
      },
      // Keep each response's upstream media selector and add a separate row for its selected item contract.
      response: function (Original, system) {
        return function NativeResponse(props) {
          const h = system.React.createElement;
          const [selected, setSelected] = system.React.useState(null);
          const panel = itemPanel(system, props, props.response, selected || props.contentType, 'Response ' + props.code + ' stream item schema', true);
          const changed = function (value) {
            setSelected(value.value);
            if (props.onContentTypeChange) props.onContentTypeChange(value);
          };
          return h(system.React.Fragment, null, h(Original, Object.assign({}, props, { onContentTypeChange: changed })),
            panel ? h('tr', { className: 'openapi-stream-row' }, h('td', { colSpan: 3 }, panel)) : null);
        };
      }
    }
  };
};
