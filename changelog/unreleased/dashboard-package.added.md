- **The dashboard is a package.** `@skelet/dashboard` publishes the dashboard compiled, with every package it uses
  left as a dependency, so a project builds it with its plugins' packages instead of carrying its source. The package
  ships a project template - a few files and a `vite.config.ts` that is `skeletDashboard()` - whose Tailwind compile
  covers the plugins' class names too, and `configure()` hands the dashboard the API and storefront URLs at runtime -
  it won't start without them. `mount()` loads the project's plugins once the dashboard's own menu exists. A dashboard
  plugin imports the registries, layout and UI kit from `@skelet/dashboard/sdk`.
