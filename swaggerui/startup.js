/* Start the offline UI with explicit configuration; query parameters may restore only registered document groups. */
/* 使用显式配置启动离线 UI；查询串只能恢复已注册的文档分类。 */
window.OpenAPIStart = function (options) {
  options.plugins = [window.OpenAPIDisplayNames];
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
