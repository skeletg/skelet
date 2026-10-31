// Package search is skelet search: query and inspect the local OpenSearch cluster without writing its DSL by hand
package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
)

const About = `Query and inspect the local OpenSearch cluster.

Commands:
  status                       cluster health, and every index with its doc count
  products <query> [flags]     search products, top 10 by default
  blog <query> [flags]         search blog posts, top 10 by default
  show products|blog <id>      every (channel, locale) document for one product or post
  raw <METHOD> <path> [body]   any REST call against the cluster

Flags for products and blog:
  -n, --limit N                number of hits (default 10)
  --json                       the raw OpenSearch response
  --channel SLUG-OR-UUID       one channel - a slug is looked up in the database
  --locale CODE                one locale, e.g. en, de

The cluster is OPENSEARCH_HOST_URL, else localhost on OPENSEARCH_HOST_PORT from your shell or the root
.env (where skelet up moves it when 9200 was taken), else localhost:9200. Reindexing lives in
skelet cli search reindex.

  skelet search products plant -n 25 --channel us --locale en
  skelet search show products 768c29f9-485a-4842-b874-dadf5f40df0b
  skelet search raw GET /_cluster/health`

// The index names and searched fields mirror internal/search/{products,blog}.go - keep them in step
type kind struct {
	index, key, title, idField string
	fields                     []string
}

var kinds = map[string]kind{
	"products": {index: "products_v1", key: "sku", title: "name", idField: "product_id", fields: []string{"name^3", "sku^2", "description"}},
	"blog":     {index: "blog_posts_v1", key: "slug", title: "title", idField: "post_id", fields: []string{"title^3", "excerpt^2", "body"}},
}

var (
	uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	slug = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

func Run(root string, args []string) int {
	cmd := "help"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	c := client{root: root, host: host(root), http: &http.Client{Timeout: 30 * time.Second}}
	var err error
	switch cmd {
	case "help", "-h", "--help", "":
		fmt.Println(About)
		return 0
	case "status":
		err = c.status()
	case "products", "blog":
		err = c.search(kinds[cmd], args)
	case "show":
		err = c.show(args)
	case "raw":
		err = c.raw(args)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n\n%s\n", cmd, About)
		return 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func host(root string) string {
	if h := os.Getenv("OPENSEARCH_HOST_URL"); h != "" {
		return h
	}
	port := os.Getenv("OPENSEARCH_HOST_PORT")
	if port == "" {
		pairs, _ := envfile.Pairs(filepath.Join(root, ".env"))
		for _, kv := range pairs {
			if v, ok := strings.CutPrefix(kv, "OPENSEARCH_HOST_PORT="); ok {
				if _, err := strconv.Atoi(v); err == nil {
					port = v
				}
			}
		}
	}
	if port == "" {
		port = "9200"
	}
	return "http://localhost:" + port
}

type client struct {
	root, host string
	http       *http.Client
}

// fetch returns the body whatever the status - an OpenSearch error is JSON worth reading - and fails only when the
// cluster can't be reached
func (c client) fetch(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, c.host+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("can't reach OpenSearch at %s: %w", c.host, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

func (c client) status() error {
	fmt.Printf("=== cluster (%s) ===\n", c.host)
	raw, err := c.fetch("GET", "/_cluster/health", nil)
	if err != nil {
		return err
	}
	var h struct {
		Status     string `json:"status"`
		Nodes      int    `json:"number_of_nodes"`
		Active     int    `json:"active_shards"`
		Unassigned int    `json:"unassigned_shards"`
	}
	if json.Unmarshal(raw, &h) == nil && h.Status != "" {
		fmt.Printf("status:  %s\nnodes:   %d\nshards:  %d active, %d unassigned\n", h.Status, h.Nodes, h.Active, h.Unassigned)
	} else {
		fmt.Println(string(raw))
	}
	fmt.Println("\n=== indices ===")
	raw, err = c.fetch("GET", "/_cat/indices?v&h=index,health,docs.count,store.size", nil)
	fmt.Print(string(raw))
	return err
}

type query struct {
	text, channel, locale string
	limit                 int
	json                  bool
}

func (c client) parse(args []string) (query, error) {
	q := query{limit: 10}
	var words []string
	value := func(i *int, flag string) (string, error) {
		if *i+1 >= len(args) {
			return "", fmt.Errorf("%s needs a value", flag)
		}
		*i++
		return args[*i], nil
	}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-n", "--limit":
			v, err := value(&i, a)
			if err != nil {
				return q, err
			}
			if q.limit, err = strconv.Atoi(v); err != nil || q.limit < 0 {
				return q, fmt.Errorf("%s takes a number, not %q", a, v)
			}
		case "--json":
			q.json = true
		case "--channel":
			v, err := value(&i, a)
			if err != nil {
				return q, err
			}
			if q.channel, err = c.channel(v); err != nil {
				return q, err
			}
		case "--locale":
			v, err := value(&i, a)
			if err != nil {
				return q, err
			}
			q.locale = v
		case "--":
			words = append(words, args[i+1:]...)
			i = len(args)
		default:
			if strings.HasPrefix(a, "-") {
				return q, fmt.Errorf("unknown flag '%s' - skelet help search", a)
			}
			words = append(words, a)
		}
	}
	if len(words) == 0 {
		return q, fmt.Errorf("missing query - skelet help search")
	}
	q.text = strings.Join(words, " ")
	return q, nil
}

// channel takes a uuid as it is and looks a slug up - checked against the slug shape first, since it goes into SQL
func (c client) channel(v string) (string, error) {
	if uuid.MatchString(v) {
		return v, nil
	}
	if !slug.MatchString(v) {
		return "", fmt.Errorf("invalid channel slug '%s' (letters, digits, _ -)", v)
	}
	out, _ := compose.Command(c.root, compose.Load(c.root).Env(), "exec", "-T", "db", "psql", "-U", "skelet", "-d", "skelet",
		"-tA", "-c", "SELECT id FROM channels WHERE slug = '"+v+"' LIMIT 1").Output()
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("no channel with slug '%s' (try a uuid, or look in skelet psql)", v)
	}
	return id, nil
}

// search mirrors the storefront resolver's query shape, so the results match the live UI
func (c client) search(k kind, args []string) error {
	q, err := c.parse(args)
	if err != nil {
		return err
	}
	filters := []any{}
	if q.channel != "" {
		filters = append(filters, map[string]any{"term": map[string]string{"channel_id": q.channel}})
	}
	if q.locale != "" {
		filters = append(filters, map[string]any{"term": map[string]string{"locale": q.locale}})
	}
	body, _ := json.Marshal(map[string]any{
		"size": q.limit,
		"query": map[string]any{"bool": map[string]any{
			"must":   []any{map[string]any{"multi_match": map[string]any{"query": q.text, "fields": k.fields, "fuzziness": "AUTO"}}},
			"filter": filters,
		}},
	})
	raw, err := c.fetch("POST", "/"+k.index+"/_search", body)
	if err != nil {
		return err
	}
	if q.json {
		return printJSON(raw)
	}
	var resp struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		fmt.Println(string(raw))
		return nil
	}
	if resp.Hits.Total.Value == 0 {
		fmt.Println("no hits")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, h := range resp.Hits.Hits {
		s := h.Source
		title := str(s[k.title])
		if title == "" {
			title = "-"
		}
		channel := str(s["channel_id"])
		if len(channel) > 8 {
			channel = channel[:8] + "…"
		}
		flag := ""
		if published, _ := s["is_published"].(bool); !published {
			flag = "[unpublished]"
		}
		fmt.Fprintf(w, "%s\t%s\tchannel:%s\tlocale:%s\t%s\n", str(s[k.key]), title, channel, str(s["locale"]), flag)
	}
	_ = w.Flush()
	fmt.Printf("- %d of %d hits\n", len(resp.Hits.Hits), resp.Hits.Total.Value)
	return nil
}

func (c client) show(args []string) error {
	if len(args) < 2 || args[0] == "" || args[1] == "" {
		return fmt.Errorf("usage: skelet search show <products|blog> <id>")
	}
	k, ok := kinds[args[0]]
	if !ok {
		return fmt.Errorf("kind must be 'products' or 'blog'")
	}
	body, _ := json.Marshal(map[string]any{"size": 100, "query": map[string]any{"term": map[string]string{k.idField: args[1]}}})
	raw, err := c.fetch("POST", "/"+k.index+"/_search", body)
	if err != nil {
		return err
	}
	var resp struct {
		Hits struct {
			Hits []struct {
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if json.Unmarshal(raw, &resp) != nil {
		fmt.Println(string(raw))
		return nil
	}
	for _, h := range resp.Hits.Hits {
		if err := printJSON(h.Source); err != nil {
			return err
		}
	}
	return nil
}

func (c client) raw(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: skelet search raw <METHOD> <path> [body]")
	}
	var body []byte
	if len(args) > 2 {
		body = []byte(args[2])
	}
	raw, err := c.fetch(args[0], args[1], body)
	if err != nil {
		return err
	}
	return printJSON(raw)
}

// printJSON indents a JSON body, and prints anything else as it came
func printJSON(raw []byte) error {
	var out bytes.Buffer
	if json.Indent(&out, raw, "", "  ") != nil {
		fmt.Println(strings.TrimRight(string(raw), "\n"))
		return nil
	}
	fmt.Println(out.String())
	return nil
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
