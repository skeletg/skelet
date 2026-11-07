- **The commerce database needs pgvector.** The bundled database moves from `postgres:18-alpine` to
  `pgvector/pgvector:0.8.6-pg18-trixie`, and the migrations create the `vector` extension. An existing volume starts
  on the new image as it is but needs one `REINDEX DATABASE` - see "Moving to the pgvector image" in `docs/deploying.md`.
  A managed database needs the extension available, which the major providers all have.
