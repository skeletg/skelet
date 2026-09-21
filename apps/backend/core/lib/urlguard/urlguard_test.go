package urlguard

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The whole point of the guard is that these never become reachable - 169.254.169.254 in particular
// hands out cloud credentials to anything that can ask
func TestDisallowedIPCoversEveryNonRoutableRange(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},             // loopback
		{"::1", true},                   // loopback v6
		{"10.0.0.1", true},              // RFC1918
		{"172.16.0.1", true},            // RFC1918
		{"192.168.1.1", true},           // RFC1918
		{"169.254.169.254", true},       // cloud metadata
		{"fe80::1", true},               // link-local v6
		{"fd00::1", true},               // unique local v6
		{"0.0.0.0", true},               // unspecified
		{"::", true},                    // unspecified v6
		{"224.0.0.1", true},             // multicast
		{"255.255.255.255", true},       // broadcast
		{"8.8.8.8", false},              // ordinary public address
		{"1.1.1.1", false},              //
		{"2606:4700:4700::1111", false}, // public v6
	} {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("%s is not parseable as an IP - fix the test", tc.ip)
		}
		if got := DisallowedIP(ip); got != tc.want {
			t.Errorf("DisallowedIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestValidateTargetURLRejectsNonHTTPSchemes(t *testing.T) {
	for _, raw := range []string{
		"ftp://example.invalid/x",
		"file:///etc/passwd",
		"gopher://example.invalid",
		"javascript:alert(1)",
		"example.invalid/webhook", // no scheme at all
		"",
	} {
		if err := ValidateTargetURL(raw, false); err == nil {
			t.Errorf("ValidateTargetURL(%q) = nil, want a scheme error", raw)
		}
		// allowPrivate is a dev convenience for the IP checks only - shape stays enforced
		if err := ValidateTargetURL(raw, true); err == nil {
			t.Errorf("ValidateTargetURL(%q, allowPrivate) = nil, want a scheme error", raw)
		}
	}
}

// Credentials in the URL would get replayed to whatever the host later resolves to, and they'd sit
// in plaintext in the admin form - reject rather than strip, so the admin sees why
func TestValidateTargetURLRejectsEmbeddedCredentials(t *testing.T) {
	for _, raw := range []string{
		"https://user:pass@8.8.8.8/hook",
		"https://user@8.8.8.8/hook",
	} {
		for _, allowPrivate := range []bool{false, true} {
			err := ValidateTargetURL(raw, allowPrivate)
			if err == nil {
				t.Fatalf("ValidateTargetURL(%q, %v) = nil, want a credentials error", raw, allowPrivate)
			}
			if !strings.Contains(err.Error(), "credentials") {
				t.Errorf("ValidateTargetURL(%q) = %v, want it to name the credentials", raw, err)
			}
		}
	}
}

func TestValidateTargetURLRequiresAHost(t *testing.T) {
	for _, raw := range []string{"http://", "https:///path"} {
		if err := ValidateTargetURL(raw, false); err == nil {
			t.Errorf("ValidateTargetURL(%q) = nil, want a missing-host error", raw)
		}
	}
}

// A literal IP skips DNS entirely, so this is the branch an attacker reaches by pasting the address
// straight into the webhook form
func TestValidateTargetURLRefusesLiteralInternalIPs(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8080/hook",
		"http://10.1.2.3/hook",
		"https://192.168.0.5/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:9000/hook",
		"http://[fd00::1]/hook",
		"http://0.0.0.0/hook",
	} {
		if err := ValidateTargetURL(raw, false); err == nil {
			t.Errorf("ValidateTargetURL(%q) = nil, want it refused as internal", raw)
		}
		// APP_ENV=development has to keep localhost callbacks working
		if err := ValidateTargetURL(raw, true); err != nil {
			t.Errorf("ValidateTargetURL(%q, allowPrivate) = %v, want nil", raw, err)
		}
	}
}

func TestValidateTargetURLAcceptsPublicTargets(t *testing.T) {
	for _, raw := range []string{
		"https://8.8.8.8/hook",
		"http://1.1.1.1:8080/hook?x=1",
		"https://[2606:4700:4700::1111]/hook",
	} {
		if err := ValidateTargetURL(raw, false); err != nil {
			t.Errorf("ValidateTargetURL(%q) = %v, want nil", raw, err)
		}
	}
}

// A name that resolves internally is refused at save time too, so the admin gets an error in the form
// rather than a webhook that silently never fires
func TestValidateTargetURLResolvesHostnamesBeforeAccepting(t *testing.T) {
	ips, err := net.LookupIP("localhost")
	if err != nil || len(ips) == 0 {
		t.Skip("localhost does not resolve here, so there is nothing to check the resolution branch against")
	}
	if err := ValidateTargetURL("http://localhost:3000/hook", false); err == nil {
		t.Error("http://localhost = nil, want it refused for resolving to a loopback address")
	}
	if err := ValidateTargetURL("http://localhost:3000/hook", true); err != nil {
		t.Errorf("http://localhost with allowPrivate = %v, want nil", err)
	}
}

// Documented behaviour: an unresolvable name passes, because the dial-time guard is the authoritative
// one and admins register endpoints before DNS exists
func TestValidateTargetURLPassesUnresolvableHosts(t *testing.T) {
	if err := ValidateTargetURL("https://not-a-real-host.invalid/hook", false); err != nil {
		t.Errorf("unresolvable host = %v, want nil so it can be saved ahead of its DNS record", err)
	}
}

// The save-time check reads DNS once - this is the layer that still holds when the record is
// re-pointed at an internal address afterwards
func TestSafeTransportRefusesToDialInternalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	guarded := &http.Client{Transport: SafeTransport(false)}
	resp, err := guarded.Get(srv.URL) // httptest always binds loopback
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("GET %s succeeded through the guarded transport, want the dial refused", srv.URL)
	}
	if !strings.Contains(err.Error(), "private or internal address") {
		t.Errorf("dial error = %v, want it to name the refusal reason", err)
	}
}

func TestSafeTransportAllowsPrivateInDevelopment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	client := &http.Client{Transport: SafeTransport(true)}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET %s with allowPrivate: %v", srv.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTeapot)
	}
}

// Cloning the default transport rather than building one from scratch is what keeps proxy and TLS
// settings intact - a bare &http.Transport{} would quietly drop them
func TestSafeTransportKeepsDefaultTransportSettings(t *testing.T) {
	def := http.DefaultTransport.(*http.Transport)
	for _, tr := range []*http.Transport{SafeTransport(false), SafeTransport(true)} {
		if tr == def {
			t.Fatal("SafeTransport returned the shared default transport, mutating it for every caller")
		}
		if tr.MaxIdleConns != def.MaxIdleConns || tr.IdleConnTimeout != def.IdleConnTimeout {
			t.Errorf("transport = {idle %d, timeout %v}, want the default's {%d, %v}",
				tr.MaxIdleConns, tr.IdleConnTimeout, def.MaxIdleConns, def.IdleConnTimeout)
		}
	}
}
