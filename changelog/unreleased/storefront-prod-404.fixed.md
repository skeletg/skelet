- **vetrina-demo's production image serves its pages again.** Every page answered 404: in production
  Next runs the proxy a second time, on the path its own rewrite pointed at, and the rule refusing a
  direct `/<channel>/` URL refused that pass too. The rewrite now carries a mark only the running
  server knows, and its second pass goes straight through - without refreshing the session or
  reporting a crawl again.
