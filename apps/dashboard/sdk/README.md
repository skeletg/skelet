# @skelet/dashboard-sdk

What a Skelet dashboard plugin imports: the registries that put it into core's pages (`registerDetailSection`,
`registerDetailField`, `registerListColumn` and the other list extensions, `registerSlot`, `registerHomeWidget`, the
nav registries), the detail-page layout and `useSaveSection`, and the dashboard's UI kit.

The package holds types alone. The dashboard's build points `@skelet/dashboard-sdk` at its own source, so a plugin
registers into the same registries and draws with the same components the dashboard's pages do. A plugin's dashboard
package takes this as a dev dependency, `react` and `@apollo/client` as peers, and ships its source:

```json
{
  "name": "@acme/fraud-dashboard",
  "exports": { ".": "./src/plugin.tsx" },
  "peerDependencies": { "react": "^19", "@apollo/client": "^4" },
  "devDependencies": { "@skelet/dashboard-sdk": "^0.5.0" }
}
```

```tsx
import { DetailPageSection, registerDetailSection } from "@skelet/dashboard-sdk";

registerDetailSection({
  id: "acme-fraud:score",
  target: "order",
  placement: "side",
  render: ({ entity }) => <FraudScore order={entity} />,
});
```

Its Go half's `skelet-plugin.json` names the package, and `skelet plugin add` installs and loads it - see
[docs/plugins.md](../../../docs/plugins.md).
