- **`skelet plugin add` installs both halves of a plugin.** A plugin names its dashboard and storefront npm packages
  in a `skelet-plugin.json`; the ops CLI fetches the Go module and records them in `plugins.json`, pnpm adds them to
  the dashboard and every storefront that loads plugins, and `skelet gen` writes each app's import list from
  `plugins.json`, so switching a plugin off unloads its UI too. `skelet plugin install` does the same for vendored
  source.
- **`@skelet/dashboard-sdk`.** A dashboard plugin package imports the registries, layout and UI kit from one name,
  which the dashboard's build points at its own source; the published package holds the types.
- **`skelet build`.** Builds what a deployment runs with the enabled plugins in: core's image as `skelet-core:prod`,
  the name compose runs, and the dashboard's static files, or whichever apps you name.
