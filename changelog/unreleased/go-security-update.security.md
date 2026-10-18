- **Go 1.27.2 and `golang.org/x/net` v0.60.0.** Fixes the standard library's HTTP/2 and HTTP/1
  desync issues, the MIME header memory limit bypass, ECH validation in `crypto/tls`, two
  `html/template` escaping bugs and `os.Root` following junctions on Windows, plus the matching
  HTTP/2 fixes in x/net. golangci-lint moves to v2.14.0 and
  `golang.org/x/tools` to v0.50.0, the first releases that read 1.27.2's export data, which gqlgen needs too.
