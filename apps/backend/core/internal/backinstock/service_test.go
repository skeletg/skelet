package backinstock

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

type captureSender struct {
	sent []email.Message
	err  error
}

func (c *captureSender) Send(_ context.Context, m email.Message) error {
	c.sent = append(c.sent, m)
	return c.err
}

// noDBService leaves q nil on purpose - any code path that reaches the database panics, which is how these tests prove a guard returned before doing work
func noDBService() *Service { return &Service{} }

// noDBWork reports a nil-database panic as the guard having let the call through
func noDBWork(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s reached the database instead of returning early (%v)", what, r)
		}
	}()
	fn()
}

func TestOnStockChangedOnlyActsOnAnIncrease(t *testing.T) { // a sale lowers stock and fires the same event, so notifying on any change would mail the waiting list every time the item sells
	pid := uuid.New()
	for name, ev := range map[string]*inventory.StockChanged{
		"a sale":           {ProductID: pid, Before: 5, After: 4, Delta: -1},
		"no net change":    {ProductID: pid, Before: 5, After: 5, Delta: 0},
		"a big write-down": {ProductID: pid, Before: 100, After: 0, Delta: -100},
		"no product":       {ProductID: uuid.Nil, Delta: 10},
	} {
		noDBWork(t, name, func() { _ = noDBService().onStockChanged(context.Background(), ev) })
	}
}

func TestOnStockChangedIgnoresOtherEvents(t *testing.T) { // the handler is attached to one topic today, but a mistyped Subscribe elsewhere must not crash the bus
	noDBWork(t, "a foreign event", func() {
		_ = noDBService().onStockChanged(context.Background(), &Requested{ChannelID: uuid.New(), ProductID: uuid.New()})
	})
}

func TestRegisterWithoutABus(t *testing.T) { // wiring skips the bus when events are off, and that must not panic at boot
	s := noDBService()
	s.Register(nil)
	if s.bus != nil {
		t.Error("a nil bus was retained")
	}
}

func TestRecordRequiresChannelProductAndEmail(t *testing.T) { // the request arrives from a public storefront form, so a blank field has to be refused before it becomes an un-notifiable row
	ch, pid := uuid.New(), uuid.New()
	for name, r := range map[string]Request{
		"no email":       {ChannelID: ch, ProductID: pid},
		"blank email":    {ChannelID: ch, ProductID: pid, Email: "   "},
		"tab email":      {ChannelID: ch, ProductID: pid, Email: "\t\n"},
		"no product":     {ChannelID: ch, Email: "a@b.test"},
		"no channel":     {ProductID: pid, Email: "a@b.test"},
		"nothing at all": {},
	} {
		var err error
		noDBWork(t, name, func() { err = noDBService().Record(context.Background(), r) })
		if err == nil {
			t.Errorf("%s: err = nil, want the request refused", name)
		}
	}
}

func TestRecordRefusesAnythingButABareAddress(t *testing.T) { // what isn't a mailbox only bounces, and every bounce costs the store's sending reputation
	ch, pid := uuid.New(), uuid.New()
	for _, bad := range []string{"not-an-email", "a@", "@b.test", "Name <a@b.test>", "a@b.test, c@d.test"} {
		var err error
		noDBWork(t, bad, func() {
			err = noDBService().Record(context.Background(), Request{ChannelID: ch, ProductID: pid, Email: bad})
		})
		if !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("%q: err = %v, want ErrInvalidEmail", bad, err)
		}
	}
}

func TestSendDirectFallsBackToTheChannelURL(t *testing.T) { // a listing can have no product path, and a mail with an empty href is a dead end for a shopper who asked to be told
	sender := &captureSender{}
	s := &Service{sender: sender}
	ch := channel.Channel{ID: uuid.New(), Name: "Vetrina", PublicURL: "https://shop.test/"}

	if err := s.sendDirect(context.Background(), "buyer@test", "", "", "", ch); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("%d messages sent, want 1", len(sender.sent))
	}
	msg := sender.sent[0]
	if !strings.Contains(msg.HTMLBody, "https://shop.test") {
		t.Errorf("HTMLBody has no link:\n%s", msg.HTMLBody)
	}
	if !strings.Contains(msg.HTMLBody, `href="https://shop.test"`) { // trimmed, since the link is also shown to the shopper in some clients
		t.Errorf("the channel URL kept its trailing slash:\n%s", msg.HTMLBody)
	}
	if !strings.Contains(msg.Subject, "is back in stock") {
		t.Errorf("Subject = %q, want it to say the item is back", msg.Subject)
	}
	if !strings.Contains(msg.Subject, "The item you wanted") {
		t.Errorf("Subject = %q, want the placeholder name rather than a bare verb", msg.Subject)
	}
	if msg.TextBody == "" {
		t.Error("TextBody is empty, so a plain-text client sees nothing")
	}
}

func TestSendDirectCarriesTheProductDetails(t *testing.T) {
	sender := &captureSender{}
	s := &Service{sender: sender}
	ch := channel.Channel{ID: uuid.New(), Name: "Vetrina", PublicURL: "https://shop.test"}

	if err := s.sendDirect(context.Background(), "buyer@test", "Winter Tire 205/55", "https://cdn.test/t.webp", "https://shop.test/p/tire", ch); err != nil {
		t.Fatal(err)
	}
	msg := sender.sent[0]

	for _, want := range []string{"Winter Tire 205/55", "https://cdn.test/t.webp", "https://shop.test/p/tire"} {
		if !strings.Contains(msg.HTMLBody, want) {
			t.Errorf("HTMLBody is missing %q:\n%s", want, msg.HTMLBody)
		}
	}
	if !strings.Contains(msg.HTMLBody, "<img") {
		t.Error("an image was supplied but no img tag was rendered")
	}
	if msg.To != "buyer@test" {
		t.Errorf("addressed To %q, want buyer@test", msg.To)
	}
	if !strings.Contains(msg.From, "Vetrina") {
		t.Errorf("From = %q, want the channel's own identity rather than a hardcoded sender", msg.From)
	}
}

func TestSendDirectOmitsTheImageBlockWhenThereIsNone(t *testing.T) { // an img with an empty src renders as a broken-image icon in most clients
	sender := &captureSender{}
	s := &Service{sender: sender}
	if err := s.sendDirect(context.Background(), "buyer@test", "A Tire", "", "https://shop.test/p", channel.Channel{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sender.sent[0].HTMLBody, "<img") {
		t.Errorf("an img tag was rendered with no image:\n%s", sender.sent[0].HTMLBody)
	}
}

func TestSendDirectEscapesTheProductName(t *testing.T) { // the name is admin-entered catalog content, so it reaches the mail body as text, never markup
	sender := &captureSender{}
	s := &Service{sender: sender}
	if err := s.sendDirect(context.Background(), "buyer@test", `<script>alert("x")</script>&`, "https://cdn.test/i.png", "https://shop.test/p", channel.Channel{}); err != nil {
		t.Fatal(err)
	}

	html := sender.sent[0].HTMLBody
	if strings.Contains(html, "<script>") {
		t.Errorf("the raw tag survived into the body:\n%s", html)
	}
	for _, want := range []string{"&lt;script&gt;", "&quot;", "&amp;"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTMLBody is missing the escaped %q:\n%s", want, html)
		}
	}
}

func TestSendDirectEscapesTheImageAndLink(t *testing.T) { // both come from catalog data the same way the name does, and unescaped they break out of the src and href attributes
	sender := &captureSender{}
	s := &Service{sender: sender}
	if err := s.sendDirect(context.Background(), "buyer@test", "A Tire",
		`https://cdn.test/i.png" onerror="alert(1)`,
		`https://shop.test/p"><script>alert(1)</script>`, channel.Channel{}); err != nil {
		t.Fatal(err)
	}

	html := sender.sent[0].HTMLBody
	if strings.Contains(html, `png" onerror`) { // the quote is what breaks out - the word itself is harmless once it is escaped
		t.Errorf("the image URL escaped its attribute:\n%s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("the link escaped its attribute:\n%s", html)
	}
	if !strings.Contains(html, "&quot;") {
		t.Errorf("HTMLBody has no escaped quotes, so neither value was escaped:\n%s", html)
	}
}

func TestSendDirectKeepsAQueryStringUsable(t *testing.T) { // escaping is in an HTML attribute, so & has to become &amp; and still resolve to the same link in a mail client
	sender := &captureSender{}
	s := &Service{sender: sender}
	if err := s.sendDirect(context.Background(), "buyer@test", "A Tire", "", "https://shop.test/p?a=1&b=2", channel.Channel{}); err != nil {
		t.Fatal(err)
	}

	html := sender.sent[0].HTMLBody
	if !strings.Contains(html, "https://shop.test/p?a=1&amp;b=2") {
		t.Errorf("the tracked link was mangled:\n%s", html)
	}
	if !strings.Contains(sender.sent[0].TextBody, "https://shop.test/p?a=1&b=2") {
		t.Errorf("the plain-text link must stay literal, not HTML-escaped:\n%s", sender.sent[0].TextBody)
	}
}

func TestSendDirectReportsASenderFailure(t *testing.T) { // the caller puts that request back to waiting, so the failure has to reach it
	boom := errors.New("smtp refused")
	sender := &captureSender{err: boom}
	s := &Service{sender: sender}
	if err := s.sendDirect(context.Background(), "buyer@test", "A Tire", "", "https://shop.test/p", channel.Channel{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the sender's failure", err)
	}
	if len(sender.sent) != 1 {
		t.Errorf("%d send attempts, want the one attempt made", len(sender.sent))
	}
}

func TestHTMLEscapeIsSinglePass(t *testing.T) { // escaping & after < would turn &lt; into &amp;lt; and show the markup as literal text in the mail
	for in, want := range map[string]string{
		"<&>":            "&lt;&amp;&gt;",
		"&<":             "&amp;&lt;",
		`"quoted"`:       "&quot;quoted&quot;",
		"a & b":          "a &amp; b",
		"already &amp;":  "already &amp;amp;",
		"plain name":     "plain name",
		"":               "",
		"205/55 R16 91V": "205/55 R16 91V",
	} {
		if got := htmlEscape(in); got != want {
			t.Errorf("htmlEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestedEventShape(t *testing.T) { // the bus routes on Type() and per-channel subscribers gate on EventChannelID()
	ch := uuid.New()
	e := &Requested{ChannelID: ch, ProductID: uuid.New(), SKU: "SKU-1", Email: "a@b.test"}
	if e.Type() != EventRequested || e.Type() != "backinstock.requested" {
		t.Errorf("Type() = %q, want backinstock.requested", e.Type())
	}
	if e.EventChannelID() != ch {
		t.Errorf("EventChannelID() = %s, want %s", e.EventChannelID(), ch)
	}
}
