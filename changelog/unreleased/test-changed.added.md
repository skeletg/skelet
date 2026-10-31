- **`skelet test --changed` runs only what a change reaches.** Go keeps its test cache, keyed on the code and on the
  migration files, so only the packages a change touches rerun; each frontend runs its typecheck and only the vitest
  files that import a changed file, and an app nothing touched is skipped. "Changed" is everything since the branch
  left develop plus what isn't committed yet, or `--changed=REF`. Every test run also starts faster: leftover test
  database clones are dropped in one go, and the database flush they cause runs in the background after the Go tests
  instead of stalling the start of the next run for a minute or more.
