- **Plugin data saves with the record.** Product and customer saves take `extensions`, each plugin's part under its
  slug, and `product.saving` / `customer.saving` hand them over inside the save's own transaction, so a plugin's rows
  commit with the record's and any plugin or sync webhook can refuse the whole save. On the dashboard, a plugin section
  stages its part with `useExtensionSection` and it goes out with the page's own Save.
