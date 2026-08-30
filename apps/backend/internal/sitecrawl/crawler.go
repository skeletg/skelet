// Package sitecrawl walks a storefront the way a search engine does - fetch the raw HTML, read the <a href> tags it
// actually contains, and never leave the origin it started on
package sitecrawl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

const (
	maxBodyBytes  = 4 << 20
	maxAnchorText = 120
	// Zero by default now that the crawler carries a rate-limit bypass token - the adaptive backoff below only
	// engages if the site still refuses, so a misconfigured bypass degrades instead of producing a garbage report
	maxDelay   = 4 * time.Second
	retryPause = 900 * time.Millisecond
)

var skipSchemes = map[string]bool{"mailto": true, "tel": true, "javascript": true, "data": true, "sms": true, "ftp": true}

// Extensions a crawler has no reason to fetch - they are never HTML and each one costs a request
var skipExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".avif": true, ".svg": true,
	".ico": true, ".css": true, ".js": true, ".mjs": true, ".json": true, ".xml": true, ".txt": true,
	".pdf": true, ".zip": true, ".gz": true, ".mp4": true, ".webm": true, ".woff": true, ".woff2": true,
}

type Options struct {
	BaseURL       string
	MaxPages      int
	MaxDepth      int
	Concurrency   int
	Delay         time.Duration
	RespectRobots bool
	UserAgent     string
	Timeout       time.Duration
	// Sent as Host when the crawl reaches the storefront on an internal address - the channel router matches on it, so
	// without it every page resolves to the wrong channel and 404s
	HostHeader string
	// Matched against SEO_CRAWL_BYPASS_TOKEN by the front API so the crawl is not rate-limited as one visitor
	BypassToken string
}

type Page struct {
	Path       string
	StatusCode int
	Title      string
	MetaDesc   string
	RobotsMeta string
	Canonical  string
	RedirectTo string
	Noindex    bool
	Nofollow   bool
	Depth      int
	FetchMS    int
	ByteSize   int
	// Heading outline in document order, so a skipped level or a missing H1 is readable rather than inferred
	Headings  []Heading
	WordCount int
	Kind      string
	OutLinks  int
	ExtLinks  int
}

type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

const (
	KindPage   = "page"
	KindFilter = "filter"
	KindParam  = "param"
)

// The listing grammar is /<entity>/f/<attr>/v/<value>.../s/<sort> - a filter chain is a facet page, which is a
// different animal for SEO than the listing it hangs off
func classifyPath(path string) string {
	base, query, hasQuery := strings.Cut(path, "?")
	for seg := range strings.SplitSeq(strings.Trim(base, "/"), "/") {
		if seg == "f" || seg == "s" {
			return KindFilter
		}
	}
	if hasQuery && query != "" {
		return KindParam
	}
	return KindPage
}

type Link struct {
	FromPath    string
	ToPath      string
	Anchor      string
	Nofollow    bool
	Occurrences int
}

// Result is one crawled page plus every in-site link found on it - Sink is called once per page, in crawl order
type Result struct {
	Page  Page
	Links []Link
}

type Crawler struct {
	opts   Options
	client *http.Client
	base   *url.URL
	robots *robots
	// Grows on every refusal and decays on clean runs - the front API rate-limits per source, and one storefront
	// container is a single bucket, so a fixed delay either crawls too slow or trips it halfway through
	delay    time.Duration
	throttle int
}

func New(opts Options) (*Crawler, error) {
	if opts.MaxPages <= 0 {
		opts.MaxPages = 500
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 12
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 20 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "SkeletLinkMap/1.0 (+internal SEO crawler)"
	}
	base, err := url.Parse(strings.TrimSpace(opts.BaseURL))
	if err != nil {
		return nil, err
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, errors.New("base url must be http or https")
	}
	if base.Host == "" {
		return nil, errors.New("base url has no host")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	base.RawQuery = ""
	base.Fragment = ""

	return &Crawler{
		opts:  opts,
		base:  base,
		delay: opts.Delay,
		client: &http.Client{
			Timeout:   opts.Timeout,
			Transport: guardedTransport(),
			// Redirects are data, not plumbing - a link pointing at a 301 is a finding, so stop and record it
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// The base url comes from an admin, so the dial is checked against the resolved address rather than the hostname -
// link-local is where cloud metadata lives and no storefront is ever served from it
func guardedTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("sitecrawl: unresolvable address %q", address)
			}
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return fmt.Errorf("sitecrawl: refusing to crawl link-local address %s", ip)
			}
			return nil
		},
	}
	return &http.Transport{DialContext: dialer.DialContext, MaxIdleConnsPerHost: 4}
}

// HitBudget reports whether the frontier still had work when the page budget ran out
type Run struct {
	Pages     int
	Links     int
	HitBudget bool
}

func (c *Crawler) Crawl(ctx context.Context, sink func(Result) error) (Run, error) {
	if c.opts.RespectRobots {
		c.robots = fetchRobots(ctx, c.client, c.base, c.opts.UserAgent, c.opts.HostHeader)
	}

	seen := map[string]bool{"/": true}
	queue := []queued{{path: "/", depth: 0}}
	run := Run{}

	for len(queue) > 0 {
		if run.Pages >= c.opts.MaxPages {
			run.HitBudget = true
			break
		}
		if err := ctx.Err(); err != nil {
			return run, err
		}
		item := queue[0]
		queue = queue[1:]

		res, discovered := c.fetch(ctx, item)
		run.Pages++
		run.Links += len(res.Links)
		if err := sink(res); err != nil {
			return run, err
		}
		if c.delay > 0 {
			select {
			case <-ctx.Done():
				return run, ctx.Err()
			case <-time.After(c.delay):
			}
		}
		if item.depth >= c.opts.MaxDepth {
			continue
		}
		for _, next := range discovered {
			if seen[next] {
				continue
			}
			seen[next] = true
			queue = append(queue, queued{path: next, depth: item.depth + 1})
		}
	}
	return run, nil
}

type queued struct {
	path  string
	depth int
}

func (c *Crawler) fetch(ctx context.Context, item queued) (Result, []string) {
	page := Page{Path: item.path, Depth: item.depth, Kind: classifyPath(item.path)}
	target := *c.base
	parsed, err := url.Parse(item.path)
	if err != nil {
		page.StatusCode = 0
		return Result{Page: page}, nil
	}
	target.Path = c.base.Path + parsed.Path
	target.RawQuery = parsed.RawQuery

	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Result{Page: page}, nil
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if c.opts.HostHeader != "" {
		req.Host = c.opts.HostHeader
	}
	if c.opts.BypassToken != "" {
		req.Header.Set("X-Skelet-Crawl", c.opts.BypassToken)
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		page.FetchMS = int(time.Since(started).Milliseconds())
		return Result{Page: page}, nil
	}
	defer resp.Body.Close()

	page.StatusCode = resp.StatusCode
	page.FetchMS = int(time.Since(started).Milliseconds())

	if loc := resp.Header.Get("Location"); loc != "" && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if to, ok := c.normalize(loc, item.path); ok {
			page.RedirectTo = to
			return Result{Page: page}, []string{to}
		}
		page.RedirectTo = loc
		return Result{Page: page}, nil
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "html") {
		return Result{Page: page}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Result{Page: page}, nil
	}
	page.ByteSize = len(body)

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return Result{Page: page}, nil
	}
	links := c.parse(doc, &page, item.path)
	page.OutLinks = len(links)
	page.WordCount = wordCount(doc)

	discovered := make([]string, 0, len(links))
	out := make([]Link, 0, len(links))
	for _, l := range links {
		out = append(out, l)
		if !page.Nofollow && !l.Nofollow && c.allowed(l.ToPath) {
			discovered = append(discovered, l.ToPath)
		}
	}
	return Result{Page: page, Links: out}, discovered
}

func (c *Crawler) refused(resp *http.Response, err error) bool {
	return err != nil || resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
}

// Back off on a refusal and recover slowly - a safety net for a site that still limits us, not a default pace
func (c *Crawler) pace(refused bool) {
	if refused {
		c.throttle++
		c.delay = min(max(c.delay*2, 100*time.Millisecond), maxDelay)
		return
	}
	c.throttle = 0
	if c.delay > c.opts.Delay {
		c.delay = max(c.opts.Delay, c.delay*3/4)
	}
}

// A 5xx or a 429 is usually the site under load rather than a real fault, so it gets one more chance before the map calls it broken
func (c *Crawler) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := c.client.Do(req)
	if !c.refused(resp, err) {
		c.pace(false)
		return resp, nil
	}
	c.pace(true)
	if resp != nil {
		resp.Body.Close()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(retryPause):
	}
	retry := req.Clone(ctx)
	resp, err = c.client.Do(retry)
	c.pace(c.refused(resp, err))
	return resp, err
}

func (c *Crawler) allowed(path string) bool {
	if c.robots == nil {
		return true
	}
	return c.robots.allows(path)
}

// normalize turns an href into a site-relative "/path?query", or reports false when it points off-origin
func (c *Crawler) normalize(href, from string) (string, bool) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return "", false
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	if ref.Scheme != "" && skipSchemes[strings.ToLower(ref.Scheme)] {
		return "", false
	}
	fromURL := *c.base
	fromParsed, err := url.Parse(from)
	if err != nil {
		return "", false
	}
	fromURL.Path = c.base.Path + fromParsed.Path
	resolved := fromURL.ResolveReference(ref)

	if resolved.Scheme != c.base.Scheme || resolved.Host != c.base.Host {
		return "", false
	}
	if c.base.Path != "" && !strings.HasPrefix(resolved.Path, c.base.Path) {
		return "", false
	}
	path := strings.TrimPrefix(resolved.Path, c.base.Path)
	if path == "" {
		path = "/"
	}
	lower := strings.ToLower(path)
	if dot := strings.LastIndex(lower, "."); dot > strings.LastIndex(lower, "/") && skipExtensions[lower[dot:]] {
		return "", false
	}
	if resolved.RawQuery != "" {
		path += "?" + resolved.RawQuery
	}
	return path, true
}

func (c *Crawler) parse(doc *html.Node, page *Page, from string) []Link {
	byTarget := map[string]*Link{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if page.Title == "" && n.FirstChild != nil {
					page.Title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "meta":
				name := strings.ToLower(attr(n, "name"))
				switch name {
				case "robots", "googlebot":
					content := attr(n, "content")
					if page.RobotsMeta == "" {
						page.RobotsMeta = content
					}
					lowered := strings.ToLower(content)
					if strings.Contains(lowered, "noindex") {
						page.Noindex = true
					}
					if strings.Contains(lowered, "nofollow") {
						page.Nofollow = true
					}
				case "description":
					if page.MetaDesc == "" {
						page.MetaDesc = strings.TrimSpace(attr(n, "content"))
					}
				}
			case "h1", "h2", "h3", "h4", "h5", "h6":
				if text := text(n); text != "" {
					page.Headings = append(page.Headings, Heading{Level: int(n.Data[1] - '0'), Text: text})
				}
			case "link":
				if strings.Contains(strings.ToLower(attr(n, "rel")), "canonical") {
					page.Canonical = attr(n, "href")
				}
			case "a":
				href := attr(n, "href")
				to, ok := c.normalize(href, from)
				if !ok && looksExternal(href) {
					page.ExtLinks++
				}
				if ok && to != from {
					existing := byTarget[to]
					if existing == nil {
						byTarget[to] = &Link{
							FromPath:    from,
							ToPath:      to,
							Anchor:      text(n),
							Nofollow:    strings.Contains(strings.ToLower(attr(n, "rel")), "nofollow"),
							Occurrences: 1,
						}
					} else {
						existing.Occurrences++
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	out := make([]Link, 0, len(byTarget))
	for _, l := range byTarget {
		out = append(out, *l)
	}
	return out
}

// Counts only what points somewhere else entirely - a fragment or a mailto is not an outbound link
func looksExternal(href string) bool {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil || ref.Host == "" {
		return false
	}
	return ref.Scheme == "" || ref.Scheme == "http" || ref.Scheme == "https"
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

// Word count needs the whole visible body - text() truncates for anchor labels, which quietly capped every page at
// a couple of words and made the thin-content check meaningless
func wordCount(root *html.Node) int {
	words := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "template", "svg":
				return
			}
		}
		if n.Type == html.TextNode {
			words += len(strings.Fields(n.Data))
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return words
}

func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	s := strings.Join(strings.Fields(b.String()), " ")
	if len(s) > maxAnchorText {
		return s[:maxAnchorText]
	}
	return s
}
