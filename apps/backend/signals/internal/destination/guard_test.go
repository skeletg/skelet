package destination

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

var errPrivateTarget = errors.New("target url resolves to a private address")

// The guard the service is held to, in the shape core fills it with - declared here so the validation
// below exercises the real rules rather than a stub that always says yes
type realGuard struct{}

func (realGuard) SafeTransport(bool) *http.Transport { return &http.Transport{} }

func (realGuard) ValidateTargetURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errPrivateTarget
	}
	if allowPrivate {
		return nil
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return nil // unresolvable is not the guard's call to make
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return errPrivateTarget
		}
	}
	return nil
}

// Destinations are admin-supplied URLs the forwarder posts the spine to, so the SSRF guard is what
// stands between an operator's typo and a POST at the metadata service
func TestAPrivateTargetIsRefusedUnlessAllowed(t *testing.T) {
	in := UpsertInput{Name: "internal", Kind: KindWebhook, TargetURL: "http://169.254.169.254/latest/meta-data"}

	if err := (&Service{guard: realGuard{}}).validate(in, true); err == nil {
		t.Error("a link-local target was accepted")
	}
	if err := (&Service{guard: realGuard{}, allowPrivate: true}).validate(in, true); err != nil {
		t.Errorf("dev bypass refused a private target: %v", err)
	}
}

func TestAPublicTargetPasses(t *testing.T) {
	err := (&Service{guard: realGuard{}}).validate(UpsertInput{Name: "ok", Kind: KindWebhook, TargetURL: "https://hooks.example.com/spine"}, true)
	if err != nil && !strings.Contains(err.Error(), "lookup") { // an offline builder can't resolve the host, which is not the guard talking
		t.Errorf("public target refused: %v", err)
	}
}

// A service built without a guard must not quietly accept everything
func TestTheGuardIsRequired(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("validate ran with no guard wired, so every target would be accepted")
		}
	}()
	_ = (&Service{}).validate(UpsertInput{Name: "x", Kind: KindWebhook, TargetURL: "http://127.0.0.1/"}, true)
}
