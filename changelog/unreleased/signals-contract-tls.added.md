- **TLS on the Signals contract.** For core and Signals on different machines,
  `SIGNALS_GRPC_TLS_CERT` and `SIGNALS_GRPC_TLS_KEY` make the suite serve its gRPC
  contract over TLS, and `SIGNALS_GRPC_TLS=true` (with `SIGNALS_GRPC_TLS_CA` for a
  private CA) makes core verify it. A renewed certificate is picked up without a
  restart. Plaintext stays the default on one private network.
