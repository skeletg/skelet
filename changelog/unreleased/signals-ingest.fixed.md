- **A bot running the storefront tracker is now flagged on every event,** not only page views,
  and its clicks and scrolls no longer reach the heatmaps or the interactions list.
- **A browser could rename a custom event or move it to another page** by sending `name` or
  `page_key` among its properties. Those now always come from the event itself.
