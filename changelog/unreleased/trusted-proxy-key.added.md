- **A storefront can run anywhere and still forward the shopper's address.** With `TRUSTED_PROXY_KEY` set on core,
  the Signals processes and the storefront, the storefront names the shopper's IP and proves it with the key, so core
  believes it whatever address the call comes from - no `TRUSTED_PROXIES` entry for a platform with no fixed egress.
  The relays from core to Signals and from ingest to core pass the resolved client on the same way, rather than
  adding their own address to the chain.
