// Report verified native rendering gaps without changing the specification or enabling requests.
window.OpenAPINativeCompatibility = function () {
  const limit = 10000;
  const diagnosticLimit = 200;

  // Inspect standard metadata positions; examples, schemas, and extensions remain opaque.
  function inspect(document) {
    const diagnostics = [];
    if (!document || !/^3\.2\.\d+$/.test(document.openapi)) return { diagnostics: diagnostics };
    let remaining = limit;
    let truncated = false;

    // Preserve source locations, HTTP method case, and actionable explanations in a stable report shape.
    function add(code, pointer, message, fix, route) {
      if (diagnostics.length >= diagnosticLimit) { truncated = true; return; }
      const diagnostic = { code: code, severity: 'warning', message: pointer + ': ' + message, fix: fix,
        source: { rule: 'swaggerui/5.32.15', kind: 'ui-limitation' } };
      if (route) diagnostic.route = route;
      diagnostics.push(diagnostic);
    }

    // Escape JSON Pointer tokens without normalizing case-sensitive names or HTTP methods.
    function token(value) { return value.replace(/~/g, '~0').replace(/\//g, '~1'); }

    // Resolve local JSON Pointers only; this inspection never retrieves external resources.
    function resolve(reference) {
      if (typeof reference !== 'string' || reference.length > 16384 || !reference.startsWith('#/')) return undefined;
      let parts;
      try { parts = decodeURIComponent(reference.slice(2)).split('/'); } catch (_) { return undefined; }
      let result = document;
      for (const part of parts) {
        const key = part.replace(/~1/g, '/').replace(/~0/g, '~');
        if (!result || typeof result !== 'object' || !Object.hasOwn(result, key)) return undefined;
        result = result[key];
      }
      return result;
    }

    // Follow referenced Path Items with a per-route cycle guard and an aggregate work bound.
    function pathItem(item, pointer, route, seen, depth = 0) {
      if (!item || typeof item !== 'object' || Array.isArray(item) || seen.has(item)) return;
      if (--remaining < 0 || depth > 64) { truncated = true; return; }
      seen.add(item);
      if (typeof item.$ref === 'string') {
        const target = resolve(item.$ref);
        if (target) pathItem(target, item.$ref, route, seen, depth + 1);
        else add('openapi.ui.reference', pointer + '/$ref', 'Rendering compatibility of this unresolved Path Item reference was not inspected.',
          'Inspect the referenced document with a compatible viewer; this check does not fetch external resources.', route);
      }
      const additional = item.additionalOperations;
      if (additional && typeof additional === 'object' && !Array.isArray(additional)) {
        for (const method of Object.keys(additional)) {
          if (--remaining < 0) { truncated = true; break; }
          const operation = additional[method];
          if (!operation || typeof operation !== 'object' || Array.isArray(operation)) continue;
          add('openapi.ui.additionalOperations', pointer + '/additionalOperations/' + token(method),
            'The operation ' + method + ' ' + route + (typeof operation.summary === 'string' ? ' (' + operation.summary + ')' : '') + ' is absent from the interactive operation list.',
            'Read this operation in the original OpenAPI document or a viewer supporting additionalOperations; submission is unavailable here.', method + ' ' + route);
        }
      }
      // Visit callback Path Items only through Operation Objects, avoiding lookalike application data.
      const operations = ['get', 'put', 'post', 'delete', 'options', 'head', 'patch', 'trace', 'query']
        .filter(method => item[method] && typeof item[method] === 'object')
        .map(method => [item[method], pointer + '/' + method, method.toUpperCase()]);
      if (additional && typeof additional === 'object') {
        for (const method of Object.keys(additional)) {
          if (--remaining < 0) { truncated = true; break; }
          operations.push([additional[method], pointer + '/additionalOperations/' + token(method), method]);
        }
      }
      for (const [operation, location, method] of operations) {
        // Report the exact inherited or operation-level parameter that the renderer cannot serialize.
        for (const [owner, ownerPointer] of [[item, pointer], [operation, location]]) {
          if (!owner || !Array.isArray(owner.parameters)) continue;
          for (let index = 0; index < owner.parameters.length; index++) {
            if (--remaining < 0) { truncated = true; return; }
            let parameter = owner.parameters[index];
            let parameterPointer = ownerPointer + '/parameters/' + index;
            const references = new Set();
            while (parameter && typeof parameter.$ref === 'string' && !references.has(parameter) && references.size < 64) {
              references.add(parameter);
              parameterPointer = parameter.$ref;
              parameter = resolve(parameter.$ref);
            }
            if (parameter && parameter.in === 'querystring') add('openapi.ui.querystring', parameterPointer,
              'The whole-query parameter is displayed but this renderer omits its value from requests. Submission for this operation is disabled.',
              'Read the content media type and encoding in the original document and submit with a compatible client.',
              method + ' ' + route);
          }
        }
        const callbacks = operation && operation.callbacks;
        if (!callbacks || typeof callbacks !== 'object') continue;
        for (const name of Object.keys(callbacks)) {
          if (--remaining < 0) { truncated = true; return; }
          let callback = callbacks[name];
          let callbackPointer = location + '/callbacks/' + token(name);
          if (callback && typeof callback.$ref === 'string') {
            callbackPointer = callback.$ref;
            callback = resolve(callback.$ref);
            if (!callback) add('openapi.ui.reference', location + '/callbacks/' + token(name) + '/$ref',
              'Rendering compatibility of this unresolved Callback reference was not inspected.',
              'Inspect the referenced document with a compatible viewer; this check does not fetch external resources.', route);
          }
          if (!callback || typeof callback !== 'object') continue;
          for (const expression of Object.keys(callback)) {
            if (--remaining < 0) { truncated = true; return; }
            if (expression === '$ref' || expression.startsWith('x-')) continue;
            pathItem(callback[expression], callbackPointer + '/' + token(expression), expression, seen, depth + 1);
          }
        }
      }
    }

    for (const field of ['paths', 'webhooks']) {
      const items = document[field];
      if (!items || typeof items !== 'object') continue;
      for (const name of Object.keys(items)) {
        if (name.startsWith('x-') && field === 'paths') continue;
        if (--remaining < 0) { truncated = true; break; }
        if (field === 'webhooks') add('openapi.ui.webhooks', '#/webhooks/' + token(name),
          'Webhook ' + JSON.stringify(name) + ' is absent from this viewer\'s operation list.',
          'Inspect the webhook Path Item and its operations in the original document or a viewer supporting webhooks.');
        pathItem(items[name], '#/' + field + '/' + token(name), name, new Set());
      }
    }
    if (Array.isArray(document.tags)) {
      for (let index = 0; index < document.tags.length; index++) {
        if (--remaining < 0) { truncated = true; break; }
        const tag = document.tags[index];
        if (!tag || typeof tag !== 'object') continue;
        const fields = ['summary', 'parent', 'kind'].filter(field => Object.hasOwn(tag, field));
        if (fields.length) add('openapi.ui.tagMetadata', '#/tags/' + index,
          'Tag ' + JSON.stringify(tag.name) + ' has ' + fields.map(field => field + ': ' + JSON.stringify(tag[field])).join(', ') + '. The renderer displays a flat name/description group and does not present these fields.',
          'Use the original document for tag labels, hierarchy, and classification; flat UI groups do not imply independent root tags.');
      }
    }
    const schemes = document.components && document.components.securitySchemes;
    if (schemes && typeof schemes === 'object' && !Array.isArray(schemes)) {
      for (const name of Object.keys(schemes)) {
        if (--remaining < 0) { truncated = true; break; }
        let scheme = schemes[name];
        let pointer = '#/components/securitySchemes/' + token(name);
        const seen = new Set();
        while (scheme && typeof scheme.$ref === 'string') {
          if (--remaining < 0 || seen.size >= 64) { truncated = true; break; }
          if (seen.has(scheme)) { scheme = undefined; break; }
          seen.add(scheme);
          const reference = scheme.$ref;
          pointer = reference;
          scheme = resolve(reference);
          if (!scheme) add('openapi.ui.reference', '#/components/securitySchemes/' + token(name) + '/$ref',
            'Rendering compatibility of this unresolved Security Scheme reference was not inspected.',
            'Inspect the referenced document with a compatible viewer; this check does not fetch external resources.');
        }
        if (!scheme || scheme.type !== 'oauth2') continue;
        if (scheme.flows && Object.hasOwn(scheme.flows, 'deviceAuthorization')) {
          add('openapi.ui.deviceAuthorization', pointer + '/flows/deviceAuthorization',
            'Security scheme ' + JSON.stringify(name) + ' displays device endpoints and scopes, but the device authorization grant is unavailable in this viewer.',
            'Use a device-authorization-capable client; the dialog intentionally offers no grant submission button.');
        }
        if (Object.hasOwn(scheme, 'oauth2MetadataUrl')) {
          add('openapi.ui.oauth2Metadata', pointer + '/oauth2MetadataUrl',
            'Security scheme ' + JSON.stringify(name) + ' displays OAuth metadata as a reference without automatic discovery.',
            'Read the metadata with your authorization client; this viewer does not fetch it or configure authorization from it.');
        }
      }
    }
    if (truncated) diagnostics.push({ code: 'openapi.ui.inspect.limit', severity: 'warning',
      message: 'Rendering compatibility inspection reached its work or diagnostic limit; remaining items were not inspected.',
      fix: 'Inspect the original OpenAPI document or split large documentation definitions.',
      source: { rule: 'swaggerui/5.32.15', kind: 'ui-limitation' } });
    return { diagnostics: diagnostics };
  }

  return {
    fn: { openapiUICompatibility: inspect },
    wrapComponents: {
      // Recompute only when the loaded immutable specification changes, including definition selection.
      InfoContainer: function (Original, system) {
        let previous;
        let report = { diagnostics: [] };
        return function NativeCompatibility(props) {
          const h = system.React.createElement;
          const document = system.specSelectors.specJson();
          if (document !== previous) {
            previous = document;
            report = inspect(document.toJS());
          }
          const original = h(Original, props);
          if (!report.diagnostics.length) return original;
          return h(system.React.Fragment, null, original,
            h('section', { className: 'openapi-compatibility', 'aria-label': 'Swagger UI compatibility' },
              h('details', null,
                h('summary', null, 'Native OpenAPI display limitations (' + report.diagnostics.length + ')'),
                h('p', null, 'The original OpenAPI document remains complete. This viewer has the following known limitations.'),
                h('ul', null, report.diagnostics.map(function (diagnostic, index) {
                  return h('li', { key: index },
                    h('code', null, diagnostic.code),
                    diagnostic.route ? h('strong', null, diagnostic.route) : null,
                    h('p', null, diagnostic.message), h('p', null, diagnostic.fix));
                })))));
        };
      }
    }
  };
};
