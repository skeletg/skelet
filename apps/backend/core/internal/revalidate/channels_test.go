package revalidate

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type channelList []StorefrontChannel

func (l channelList) StorefrontChannels(context.Context) ([]StorefrontChannel, error) { return l, nil }

type hostedPurge struct {
	host     string
	channels []string
	warm     []string
}

// One listener answering every storefront host, so each public URL can be told apart by the Host it was sent to
func multiHostStorefront(t *testing.T) (*Client, *[]hostedPurge) {
	t.Helper()
	var mu sync.Mutex
	got := &[]hostedPurge{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p struct {
			Channels []string `json:"channels"`
			Warm     []string `json:"warm"`
		}
		_ = json.Unmarshal(body, &p)
		mu.Lock()
		*got = append(*got, hostedPurge{host: r.Host, channels: p.Channels, warm: p.Warm})
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]int{"warmed": len(p.Warm) * len(p.Channels)})
	}))
	t.Cleanup(srv.Close)
	c := New("", "secret", nil)
	c.http = &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", srv.Listener.Addr().String())
	}}}
	return c, got
}

func TestEachChannelsPurgeReachesItsOwnStorefront(t *testing.T) {
	us, eu, none, off := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	channels := channelList{
		{ID: us, PublicURL: "https://us.shop.test", IsActive: true},
		{ID: eu, PublicURL: "https://eu.shop.test/", IsActive: true},
		{ID: none, PublicURL: "http://localhost:3000", IsActive: true},
		{ID: off, PublicURL: "https://off.shop.test", IsActive: false},
	}
	got := map[string][]string{}
	for _, tg := range buildTargets(channels, "") {
		got[tg.url] = tg.channels
	}
	want := map[string][]string{
		"https://us.shop.test/api/revalidate": {us.String()},
		"https://eu.shop.test/api/revalidate": {eu.String()},
	}
	if len(got) != len(want) {
		t.Fatalf("targets = %v, want %v - each active channel at its own storefront, none for a deactivated or unreachable one", got, want)
	}
	for u, ids := range want {
		if !slices.Equal(got[u], ids) {
			t.Errorf("%s serves %v, want %v", u, got[u], ids)
		}
	}

	pooled := buildTargets(channels, "http://storefront:3000/api/revalidate")
	all := []string{us.String(), eu.String(), none.String()}
	slices.Sort(all)
	if len(pooled) != 1 || pooled[0].url != "http://storefront:3000/api/revalidate" || !slices.Equal(pooled[0].channels, all) {
		t.Fatalf("pooled targets = %+v, want the configured storefront for every active channel", pooled)
	}
}

func TestAPurgeIsSentOncePerStorefrontWithTheChannelsItServes(t *testing.T) {
	us, eu := uuid.New(), uuid.New()
	c, got := multiHostStorefront(t)
	c.SetChannels(channelList{
		{ID: us, PublicURL: "http://shops.test", IsActive: true},
		{ID: eu, PublicURL: "http://shops.test/eu", IsActive: true},
	})
	if err := c.warmPath(context.Background(), "", nil, "/shoes"); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if len(*got) != 2 {
		t.Fatalf("sent %d requests, want one purge and one warm to the storefront both channels share", len(*got))
	}
	ids := []string{us.String(), eu.String()}
	slices.Sort(ids)
	for _, p := range *got {
		if p.host != "shops.test" || !slices.Equal(p.channels, ids) {
			t.Fatalf("request went to %q for %v, want shops.test for %v", p.host, p.channels, ids)
		}
	}
}

func TestALoopbackPublicURLIsNotAPurgeTarget(t *testing.T) {
	for _, u := range []string{"http://localhost:3000", "http://127.0.0.1:5174", "http://[::1]:3000", "not a url", ""} {
		if got := storefrontEndpoint(u); got != "" {
			t.Errorf("storefrontEndpoint(%q) = %q, want none", u, got)
		}
	}
	if got := storefrontEndpoint("https://shop.example.com/any/path"); got != "https://shop.example.com/api/revalidate" {
		t.Errorf("storefrontEndpoint = %q", got)
	}
}

func TestAPooledFanOutIsChunkedAndBounded(t *testing.T) {
	var (
		mu             sync.Mutex
		inFlight, peak int
		seen           []string
		chunks         [][]string
	)
	failing := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Channels []string `json:"channels"`
			Warm     []string `json:"warm"`
		}
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		seen = append(seen, p.Channels...)
		chunks = append(chunks, p.Channels)
		fail := slices.Contains(p.Channels, failing)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"warmed": len(p.Warm) * len(p.Channels)})
	}))
	t.Cleanup(srv.Close)
	var chs channelList
	for range 30 {
		chs = append(chs, StorefrontChannel{ID: uuid.New(), IsActive: true})
	}
	failing = chs[0].ID.String()
	c := New(srv.URL, "secret", nil)
	c.SetChannels(chs)
	warmed, err := c.postAll(context.Background(), nil, nil, []string{"/"})
	if len(seen) != 30 || warmed < 30-channelsPerPost {
		t.Fatalf("reached %d channels, warmed %d, want all 30 across chunks", len(seen), warmed)
	}
	for _, ch := range chunks {
		if len(ch) > channelsPerPost {
			t.Fatalf("one request carried %d channels, want at most %d", len(ch), channelsPerPost)
		}
	}
	if peak > postConcurrency || peak < 2 {
		t.Fatalf("peak concurrency %d, want between 2 and %d", peak, postConcurrency)
	}
	if err == nil || !strings.Contains(err.Error(), failing) {
		t.Fatalf("err = %v, want it to name the failing chunk's channels", err)
	}
}

func TestAHangingStorefrontCannotOutlastTheSendBudget(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	old := sendBudget
	sendBudget = 100 * time.Millisecond
	t.Cleanup(func() { sendBudget = old })
	c := New(srv.URL, "secret", nil)
	start := time.Now()
	if err := c.warmPath(context.Background(), "", nil, "/slow"); err == nil {
		t.Fatal("a hung warm reported success")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("warm took %v, want it cut at the budget", d)
	}
}
