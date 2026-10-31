- **The scripts behind `skelet gen` and `skelet lint` are gone** - both are part of the CLI now, and
  `skelet check codegen` is `skelet gen --check`. The golangci-lint version they pin lives in the CLI's
  source, where `skelet check pins` and Renovate find it.
