// Present native security metadata without changing definitions or starting discovery requests.
window.OpenAPINativeSecurity = function () {
  // Follow bounded local references; rendered endpoints always come from the original document.
  function securityScheme(document, name) {
    if (!document || !/^3\.2\.\d+$/.test(document.openapi)) return undefined;
    const definitions = document.components && document.components.securitySchemes;
    if (!definitions || !Object.hasOwn(definitions, name)) return undefined;
    let scheme = definitions[name];
    const seen = new Set();
    while (scheme && typeof scheme === 'object' && !Array.isArray(scheme)) {
      if (typeof scheme.$ref !== 'string') return scheme;
      if (seen.has(scheme) || seen.size >= 64) return undefined;
      seen.add(scheme);
      const reference = scheme.$ref;
      if (reference.length > 16384 || !reference.startsWith('#/')) return undefined;
      let parts;
      try { parts = decodeURIComponent(reference.slice(2)).split('/'); } catch (_) { return undefined; }
      scheme = document;
      for (const part of parts) {
        const key = part.replace(/~1/g, '/').replace(/~0/g, '~');
        if (!scheme || typeof scheme !== 'object' || !Object.hasOwn(scheme, key)) return undefined;
        scheme = scheme[key];
      }
    }
    return undefined;
  }

  // Reuse the immutable document between dialog renders and reset when definitions change.
  function wrapAuthorization(Original, system) {
    let previous;
    let document;
    return function NativeAuthorization(props) {
      const current = system.specSelectors.specJson();
      if (current !== previous) { previous = current; document = current.toJS(); }
      const flow = props.schema && props.schema.get('flow');
      let scheme = securityScheme(document, props.name);
      // A missing local definition must not restore upstream's unsupported device grant action.
      if (!scheme && document && /^3\.2\.\d+$/.test(document.openapi) && flow === 'deviceAuthorization') scheme = { type: 'oauth2' };
      const h = system.React.createElement;
      const original = h(Original, props);
      if (!scheme) return original;
      const metadata = [];
      if (scheme.deprecated === true) metadata.push(h('p', { key: 'deprecated', className: 'openapi-security-deprecated' }, 'Deprecated security scheme'));
      if (scheme.type === 'oauth2' && typeof scheme.oauth2MetadataUrl === 'string') {
        metadata.push(h('p', { key: 'metadata' }, 'OAuth metadata URL: ', h('code', null, scheme.oauth2MetadataUrl)));
        metadata.push(h('p', { key: 'discovery' }, 'Metadata is shown for reference. This viewer does not retrieve it or configure authorization from it.'));
      }
      if (scheme.type !== 'oauth2' || flow !== 'deviceAuthorization') {
        if (!metadata.length) return original;
        return h('div', { className: 'openapi-security', 'aria-label': props.name + ' security scheme' }, original, metadata);
      }
      const device = scheme.flows && scheme.flows.deviceAuthorization || {};
      const Markdown = system.getComponent('Markdown', true);
      const scopes = props.schema.get('allowedScopes') || props.schema.get('scopes');
      const entries = scopes ? scopes.entrySeq().toArray() : [];
      return h('section', { className: 'openapi-security', 'aria-label': props.name + ' device authorization' },
        h('h4', null, props.name, ' (OAuth2, deviceAuthorization)'),
        scheme.description ? h(Markdown, { source: scheme.description }) : null,
        metadata,
        ['deviceAuthorizationUrl', 'tokenUrl', 'refreshUrl'].filter(key => typeof device[key] === 'string').map(key =>
          h('p', { key: key }, ({ deviceAuthorizationUrl: 'Device authorization URL', tokenUrl: 'Token URL', refreshUrl: 'Refresh URL' })[key] + ': ', h('code', null, device[key]))),
        h('p', { className: 'openapi-wire-note' }, 'Device authorization is not available in this viewer. Use a client supporting the device authorization grant with the endpoints and scopes shown here.'),
        entries.length ? h('div', null, h('h5', null, 'Scopes'), h('dl', null, entries.map(([name, description]) =>
          h(system.React.Fragment, { key: name }, h('dt', null, h('code', null, name)), h('dd', null, description))))) : null,
        h('div', { className: 'auth-btn-wrapper' }, h('button', { type: 'button', className: 'btn modal-btn auth btn-done',
          onClick: function () { system.authActions.showDefinitions(false); } }, 'Close')));
    };
  }

  return { fn: { openapiSecurityScheme: securityScheme }, wrapComponents: { oauth2: wrapAuthorization, AuthItem: wrapAuthorization } };
};
