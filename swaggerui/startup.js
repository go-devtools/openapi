/* Start the offline UI with explicit configuration; query parameters may restore only registered document groups. */
window.OpenAPIStart = function (options) {
  options.plugins = [window.OpenAPIDisplayNames, window.OpenAPINativeExamples, window.OpenAPINativeCompatibility, window.OpenAPINativeWire];
  if (options.urls) {
    options.presets = [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset];
    const selected = new URL(window.location.href).searchParams.get("urls.primaryName");
    if (options.urls.some((definition) => definition.name === selected)) {
      options["urls.primaryName"] = selected;
    }
  }
  options.oauth2RedirectUrl = new URL("./oauth2-redirect.html", window.location.href).href;
  window.ui = SwaggerUIBundle(options);
};
