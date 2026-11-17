- **Plugins reach more of the dashboard.** A plugin's detail section can sit at the top of a record's page, at the
  end or in its side panel, and sections now render on every record page, categories, blog posts, shipping zones and
  the rest included. Every top-level list takes plugin columns, filters and actions, with zones of its own above and
  below the table, and `registerHomeWidget` adds a section to Home that admins arrange like core's.
- **Plugins reach more of the storefront.** The plugin registry moved into `@skelet/storefront-react`, so a plugin's
  storefront package registers into it from npm, and Vetrina draws zones in the header, footer, listings, product
  cards, product page, mini cart, checkout, order pages and account overview, each handed what the page knows.
