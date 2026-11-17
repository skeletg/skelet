# @skelet/dashboard

The Skelet admin dashboard, compiled. A project builds it with its plugins' dashboard packages, so the dashboard is a
version in its `package.json` rather than code it carries.

Start from `template/`: a page shell, `src/main.ts`, `src/config.ts` reading the project's `.env`, `src/plugins.ts`
importing the plugins' packages, `src/index.css`,
and a `vite.config.ts` that is `skeletDashboard()` from `@skelet/dashboard/vite`. `vite build` makes the static files
any host serves; `_headers` and `_redirects` ride along for the hosts that read them.

- `@skelet/dashboard` - `mount(el)`, imported dynamically after `configure()` and the plugins, so it can't load first
- `@skelet/dashboard/config` - `configure({ graphqlUrl, storefrontOrigin, ... })`
- `@skelet/dashboard/sdk` - what a plugin imports: the registries, the detail-page layout, `useSaveSection`, the UI kit
- `@skelet/dashboard/vite` - the project's Vite config
- `@skelet/dashboard/styles.css` - the Tailwind source, compiled by the project with its plugins' class names

A plugin's dashboard package imports from `@skelet/dashboard/sdk`, takes `@skelet/dashboard`, `react` and
`@apollo/client` as peers, and ships its source:

```tsx
import { registerDetailSection } from "@skelet/dashboard/sdk";

registerDetailSection({
  id: "acme-fraud:score",
  target: "order",
  placement: "side",
  render: ({ entity }) => <FraudScore order={entity} />,
});
```

GPL-3.0 with the [Skelet Plugin Exception](PLUGIN-EXCEPTION.txt): a plugin built against it may take any license.
See [docs/plugins.md](https://github.com/skeletg/skelet/blob/main/docs/plugins.md).
