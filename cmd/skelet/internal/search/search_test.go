package search

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// A cluster that answers every search with hits, and records what it was asked
func cluster(t *testing.T, hits string) *[]string {
	t.Helper()
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.Method+" "+r.URL.RequestURI()+" "+string(raw))
		switch {
		case r.URL.Path == "/_cluster/health":
			_, _ = w.Write([]byte(`{"status":"green","number_of_nodes":1,"active_shards":4,"unassigned_shards":0}`))
		case strings.HasPrefix(r.URL.Path, "/_cat/"):
			_, _ = w.Write([]byte("index health docs.count\nproducts_v1 green 12\n"))
		default:
			_, _ = w.Write([]byte(hits))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OPENSEARCH_HOST_URL", srv.URL)
	return &bodies
}

func stdout(t *testing.T, f func() int) (string, int) {
	t.Helper()
	r, w, _ := os.Pipe()
	orig := os.Stdout
	os.Stdout = w
	code := f()
	os.Stdout = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out), code
}

const twoHits = `{"hits":{"total":{"value":7},"hits":[
	{"_source":{"sku":"PLANT-1","name":"Fern","channel_id":"56d051bc-0131-4476","locale":"en","is_published":true}},
	{"_source":{"sku":"PLANT-2","channel_id":"56d051bc-0131-4476","locale":"de","is_published":false}}]}}`

func TestSearchSendsTheStorefrontsQuery(t *testing.T) {
	bodies := cluster(t, twoHits)
	out, code := stdout(t, func() int {
		return Run(t.TempDir(), []string{"products", `fern "big"`, "pot", "-n", "25", "--locale", "en", "--channel", "56d051bc-0131-4476-bc0b-121d80290684"})
	})
	if code != 0 || len(*bodies) != 1 || !strings.HasPrefix((*bodies)[0], "POST /products_v1/_search ") {
		t.Fatalf("exit %d, requests %q", code, *bodies)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN((*bodies)[0], " ", 3)[2]), &got); err != nil {
		t.Fatal(err)
	}
	want := `{"query":{"bool":{"filter":[{"term":{"channel_id":"56d051bc-0131-4476-bc0b-121d80290684"}},{"term":{"locale":"en"}}],` +
		`"must":[{"multi_match":{"fields":["name^3","sku^2","description"],"fuzziness":"AUTO","query":"fern \"big\" pot"}}]}},"size":25}`
	if b, _ := json.Marshal(got); string(b) != want {
		t.Errorf("sent %s\nwant %s", b, want)
	}
	for _, line := range []string{"PLANT-1  Fern  channel:56d051bc…  locale:en", "PLANT-2  -     channel:56d051bc…  locale:de  [unpublished]", "- 2 of 7 hits"} {
		if !strings.Contains(out, line) {
			t.Errorf("no %q in:\n%s", line, out)
		}
	}
}

func TestSearchEdges(t *testing.T) {
	cluster(t, `{"hits":{"total":{"value":0},"hits":[]}}`)
	if out, _ := stdout(t, func() int { return Run(t.TempDir(), []string{"blog", "nothing"}) }); strings.TrimSpace(out) != "no hits" {
		t.Errorf("no hits printed %q", out)
	}
	if out, _ := stdout(t, func() int { return Run(t.TempDir(), []string{"blog", "x", "--json"}) }); !strings.Contains(out, "\n  \"hits\": {") {
		t.Errorf("--json isn't indented JSON: %q", out)
	}
	if out, _ := stdout(t, func() int { return Run(t.TempDir(), []string{"status"}) }); !strings.Contains(out, "status:  green") || !strings.Contains(out, "products_v1 green 12") {
		t.Errorf("status:\n%s", out)
	}
	for _, args := range [][]string{{"products"}, {"products", "x", "-n", "lots"}, {"products", "x", "--bogus"}, {"show", "orders", "1"}, {"raw", "GET"}, {"nope"}} {
		if _, code := stdout(t, func() int { return Run(t.TempDir(), args) }); code != 1 {
			t.Errorf("%q: exit %d, want 1", args, code)
		}
	}
}

func TestShowAndRaw(t *testing.T) {
	bodies := cluster(t, twoHits)
	out, code := stdout(t, func() int { return Run(t.TempDir(), []string{"show", "blog", "abc"}) })
	if code != 0 || !strings.Contains((*bodies)[0], `"term":{"post_id":"abc"}`) || strings.Count(out, `"locale"`) != 2 {
		t.Errorf("exit %d, sent %q, printed:\n%s", code, *bodies, out)
	}
	if _, code := stdout(t, func() int { return Run(t.TempDir(), []string{"raw", "PUT", "/x", `{"a":1}`}) }); code != 0 || (*bodies)[1] != `PUT /x {"a":1}` {
		t.Errorf("raw: exit %d, sent %q", code, (*bodies)[1])
	}
}

func TestUnreachableClusterFails(t *testing.T) {
	t.Setenv("OPENSEARCH_HOST_URL", "http://127.0.0.1:1")
	if _, code := stdout(t, func() int { return Run(t.TempDir(), []string{"status"}) }); code != 1 {
		t.Errorf("exit %d against nothing", code)
	}
}

func TestHostFollowsTheMovedPort(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/.env", []byte("OPENSEARCH_HOST_PORT=9300\nOPENSEARCH_HOST_PORT=9310\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENSEARCH_HOST_URL", "")
	t.Setenv("OPENSEARCH_HOST_PORT", "")
	if got := host(root); got != "http://localhost:9310" {
		t.Errorf("host = %s", got)
	}
	t.Setenv("OPENSEARCH_HOST_PORT", "9400")
	if got := host(root); got != "http://localhost:9400" {
		t.Errorf("with the env set: %s", got)
	}
}

// A fake docker answering the channel lookup: a uuid for the slug "us", nothing for anything else. Every call is logged,
// so a test can tell a slug that never reached SQL from one that found no channel
func fakeDocker(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := dir + "/calls"
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \"$*\" in *\"slug = 'us'\"*) echo 9ead3bed-0000-4000-8000-000000000001;; esac\n"
	if err := os.WriteFile(dir+"/docker", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestChannelLookup(t *testing.T) {
	log := fakeDocker(t)
	c := client{root: t.TempDir()}
	if _, err := c.channel("us'; DROP TABLE channels; --"); err == nil || !strings.Contains(err.Error(), "invalid channel slug") {
		t.Errorf("a slug with SQL in it: %v", err)
	}
	if raw, _ := os.ReadFile(log); len(raw) != 0 {
		t.Errorf("an invalid slug reached the database: %s", raw)
	}
	if id, err := c.channel("us"); err != nil || id != "9ead3bed-0000-4000-8000-000000000001" {
		t.Errorf("slug us: %q, %v", id, err)
	}
	if _, err := c.channel("nowhere"); err == nil || !strings.Contains(err.Error(), "no channel with slug") {
		t.Errorf("an unknown slug: %v", err)
	}
	before, _ := os.ReadFile(log)
	if id, err := c.channel("56d051bc-0131-4476-bc0b-121d80290684"); err != nil || id != "56d051bc-0131-4476-bc0b-121d80290684" {
		t.Errorf("a uuid: %q, %v", id, err)
	}
	if after, _ := os.ReadFile(log); len(after) != len(before) {
		t.Error("a uuid was looked up instead of used as it is")
	}
}
