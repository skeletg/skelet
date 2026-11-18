- **Plugins filter core's lists by their own data.** `RegisterListFilter` puts a filter on the products, customers or
  orders list - "has a warranty", "ERP sync failed" - and the dashboard shows it in the list's toolbar on its own. Core
  narrows the page in SQL to the records the plugin matched, so counts and paging stay right.
