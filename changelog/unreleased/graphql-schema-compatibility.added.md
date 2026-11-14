- **`skelet check` holds the admin and storefront GraphQL schemas compatible with `main`.** A removed field, an output
  that can now be null, a new required argument or input field fails it, since a dashboard or storefront deployed
  apart from core may run against an older or newer one. Deprecate first and remove in a later release, or list a
  break taken on purpose in `apps/backend/core/graph/schema/accepted-breaks`.
