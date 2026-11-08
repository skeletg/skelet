- **The dashboard's default look follows the system.** An admin who hasn't picked a theme gets
  light or dark to match their operating system, docked to the dark rail with tighter corners.
  A theme someone already chose stays as it is. The Skelet Modern preset is that same look.
- **The admin search (⌘K) finds orders, customers and products,** not just pages: an order by its
  number, email or the name it ships to, a customer by email or full name, a product by name or
  SKU, from three typed characters. Each kind only shows to an admin who may read it, in the
  channels they may read it in. It runs on a new `adminSearch` query that returns a few light rows
  per kind, backed by new trigram indexes on orders and customers. The palette also opens larger,
  with each result's icon, where it lives, and the keys spelled out underneath.
- **The sidebar puts daily work first.** Orders, Customers and Catalog come straight after the
  Dashboard, and the setup pages (Channels, Admins, Developer, Import & export, Settings) sit at the
  bottom. The settings page's sidebar entry is now "All settings".
- **Settings is grouped:** Store (All settings, Privacy), Checkout & payments (Checkout, Payment
  providers, Tax rules) and Shipping (zones, boxes). Search engine moved under Catalog and Cache
  under Developer, and the privacy and cache settings in All settings say they're also on their own
  pages.
