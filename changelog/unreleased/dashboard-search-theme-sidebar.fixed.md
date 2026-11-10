- **The GraphQL playground no longer takes the dashboard down.** Opening its Operations or Values
  panel replaced the whole dashboard with an error page, and its editor had stopped validating
  queries in the dev server. Both work again.
- **Searching orders by `_` or `%` no longer matches every order.** The orders list took them as
  wildcards.
- **An open order page no longer blanks itself** when the orders list loads in another tab. The
  list's shorter copy of the shipping address replaced the page's full one in the cache.
