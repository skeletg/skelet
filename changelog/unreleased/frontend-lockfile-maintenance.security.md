- **Frontend lockfiles refresh weekly.** Renovate's lockfile maintenance re-resolves the
  dashboard's and each storefront's transitive dependencies within their ranges, so a patched
  release reaches them without an override. Both lockfiles are refreshed now, which clears the
  markdown-it advisory and drops every audit override but `@graphql-tools/utils`, whose parent
  still excludes the fix.
