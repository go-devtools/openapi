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

  // Render one independently validated stream item, preserving boolean schemas and reference siblings.
  function itemPanel(system, props, owner, contentType, label) {
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
      h('p', null, 'Applies independently to each item in this sequence.'), visual);
  }

  return {
    fn: { openapiHasWholeQuery: hasWholeQuery },
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
      // Follow request media changes without changing the complete-body schema or wire editor.
      RequestBody: function (Original, system) {
        return function NativeRequestBody(props) {
          const h = system.React.createElement;
          return h(system.React.Fragment, null, h(Original, props),
            itemPanel(system, props, props.requestBody, props.contentType, 'Request stream item schema'));
        };
      },
      // Keep each response's upstream media selector and add a separate row for its selected item contract.
      response: function (Original, system) {
        return function NativeResponse(props) {
          const h = system.React.createElement;
          const [selected, setSelected] = system.React.useState(null);
          const panel = itemPanel(system, props, props.response, selected || props.contentType, 'Response ' + props.code + ' stream item schema');
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
