package auth

import (
	"net/http/httputil"
	"net/url"
	"slices"
)

func Relay(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.Out.Host = pr.In.Host
		if _, ok := pr.Out.Header["User-Agent"]; !ok {
			pr.Out.Header.Set("User-Agent", "")
		}
		for _, h := range []string{"X-Forwarded-Host", "X-Forwarded-Proto"} {
			if v := pr.In.Header.Values(h); len(v) > 0 {
				pr.Out.Header[h] = slices.Clone(v)
			}
		}
		pr.Out.Header.Del(ClientIPHeader)
		pr.Out.Header.Del(ProxyKeyHeader)
		ip := ClientIP(pr.In)
		if ip == "" {
			return
		}
		pr.Out.Header.Set("X-Forwarded-For", ip)
		if key := currentProxyKey(); key != "" {
			pr.Out.Header.Set(ClientIPHeader, ip)
			pr.Out.Header.Set(ProxyKeyHeader, key)
		}
	}}
}
