package sitecrawl

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Only the Disallow/Allow subset matters here - the crawl already refuses to leave the origin, so host and
// sitemap directives have nothing to add
type robots struct {
	allow    []string
	disallow []string
}

func fetchRobots(ctx context.Context, client *http.Client, base *url.URL, userAgent, hostHeader string) *robots {
	target := *base
	target.Path = "/robots.txt"
	target.RawQuery = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", userAgent)
	if hostHeader != "" {
		req.Host = hostHeader
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return nil
	}
	return parseRobots(string(body))
}

// A group only counts once its User-agent block is the wildcard one - a rule aimed at GPTBot is not ours to obey
func parseRobots(body string) *robots {
	r := &robots{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	inWildcard := false
	sawAgent := false
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if !sawAgent || !inWildcard {
				inWildcard = value == "*"
			} else if value != "*" {
				inWildcard = false
			}
			sawAgent = true
		case "disallow":
			if inWildcard && value != "" {
				r.disallow = append(r.disallow, value)
			}
		case "allow":
			if inWildcard && value != "" {
				r.allow = append(r.allow, value)
			}
		}
	}
	if len(r.allow) == 0 && len(r.disallow) == 0 {
		return nil
	}
	return r
}

// Longest match wins, which is what every real crawler does with overlapping Allow and Disallow
func (r *robots) allows(path string) bool {
	if r == nil {
		return true
	}
	best := ""
	permitted := true
	for _, rule := range r.disallow {
		if matchRule(rule, path) && len(rule) > len(best) {
			best, permitted = rule, false
		}
	}
	for _, rule := range r.allow {
		if matchRule(rule, path) && len(rule) >= len(best) {
			best, permitted = rule, true
		}
	}
	return permitted
}

func matchRule(rule, path string) bool {
	if !strings.ContainsAny(rule, "*$") {
		return strings.HasPrefix(path, rule)
	}
	anchored := strings.HasSuffix(rule, "$")
	rule = strings.TrimSuffix(rule, "$")
	parts := strings.Split(rule, "*")
	rest := path
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(rest, part)
		if idx < 0 || (i == 0 && idx != 0) {
			return false
		}
		rest = rest[idx+len(part):]
	}
	return !anchored || rest == ""
}
