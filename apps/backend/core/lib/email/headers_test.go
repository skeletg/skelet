package email

import (
	"strings"
	"testing"
)

func TestListUnsubscribeHeadersNeedAnAbsoluteURL(t *testing.T) {
	h := ListUnsubscribeHeaders("https://shop.test/unsubscribe/tok")
	if h["List-Unsubscribe"] != "<https://shop.test/unsubscribe/tok>" {
		t.Errorf("List-Unsubscribe = %q", h["List-Unsubscribe"])
	}
	if h["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Errorf("List-Unsubscribe-Post = %q, want the RFC 8058 one-click value", h["List-Unsubscribe-Post"])
	}
	for _, u := range []string{"", "/unsubscribe/tok", "mailto:x@y.test"} {
		if h := ListUnsubscribeHeaders(u); h != nil {
			t.Errorf("ListUnsubscribeHeaders(%q) = %v, want none", u, h)
		}
	}
}

func TestTheWireMessageCarriesExtraHeadersAndNoInjectedOnes(t *testing.T) {
	raw := string(buildMIMEMessage(Message{
		From: "a@shop.test", To: "b@shop.test", Subject: "Hi",
		Headers: map[string]string{
			"List-Unsubscribe":      "<https://shop.test/unsubscribe/tok>",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click\r\nBcc: victim@evil.test",
		},
	}))
	head, _, _ := strings.Cut(raw, "\r\n\r\n")
	if !strings.Contains(head, "\r\nList-Unsubscribe: <https://shop.test/unsubscribe/tok>\r\n") {
		t.Errorf("List-Unsubscribe missing from the header block:\n%s", head)
	}
	if strings.Contains(head, "\r\nBcc:") {
		t.Errorf("a CRLF in a header value started a header of its own:\n%s", head)
	}
}

func TestSignupConfirmationCarriesTheLink(t *testing.T) {
	subj, html, text, err := RenderSignupConfirmationEmail(SignupConfirmationEmailData{
		Link: "https://shop.test/subscribe/confirm/tok", StoreName: "Shop", ExpiresIn: "7 days",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subj == "" || !strings.Contains(html, "https://shop.test/subscribe/confirm/tok") || !strings.Contains(text, "https://shop.test/subscribe/confirm/tok") {
		t.Errorf("confirmation rendered without its link: %q\n%s\n%s", subj, html, text)
	}
	if !strings.Contains(text, "Hi there") {
		t.Errorf("an anonymous subscriber got no greeting fallback:\n%s", text)
	}
}

// The core headers went out raw - a line break in a sender name or subject started a header of its own
func TestCoreHeadersCannotBeInjectedAndAreEncoded(t *testing.T) {
	raw := string(buildMIMEMessage(Message{
		From:    "\"Shop\r\nBcc: victim@evil.test\" <a@shop.test>",
		To:      "b@shop.test\r\nBcc: victim@evil.test",
		Subject: "Porudžbina 1001\r\nBcc: victim@evil.test",
	}))
	head, _, _ := strings.Cut(raw, "\r\n\r\n")
	if strings.Contains(head, "\r\nBcc:") {
		t.Fatalf("a CRLF in a core header started a header of its own:\n%s", head)
	}
	if !strings.Contains(head, "Subject: =?utf-8?q?") {
		t.Errorf("a non-ASCII subject went out unencoded:\n%s", head)
	}
	plain := string(buildMIMEMessage(Message{From: "a@shop.test", To: "b@shop.test", Subject: "Order 1001"}))
	if !strings.Contains(plain, "\r\nSubject: Order 1001\r\n") {
		t.Errorf("a plain ASCII subject was rewritten:\n%s", plain)
	}
}

// A fixed boundary let body text containing it forge or cut off a part
func TestTheBoundaryIsUnguessable(t *testing.T) {
	a := string(buildMIMEMessage(Message{From: "a@shop.test", To: "b@shop.test", TextBody: "--skelet-mime-boundary"}))
	b := string(buildMIMEMessage(Message{From: "a@shop.test", To: "b@shop.test"}))
	ba, bb := boundaryOf(t, a), boundaryOf(t, b)
	if ba == bb || ba == "skelet-mime-boundary" {
		t.Fatalf("boundaries %q and %q - want a fresh random one per message", ba, bb)
	}
	if strings.Count(a, "--"+ba+"\r\n") != 2 || !strings.Contains(a, "--"+ba+"--") {
		t.Errorf("the message doesn't split into its two parts on %q", ba)
	}
}

func boundaryOf(t *testing.T, raw string) string {
	t.Helper()
	_, rest, ok := strings.Cut(raw, `boundary="`)
	if !ok {
		t.Fatalf("no boundary in:\n%s", raw)
	}
	b, _, _ := strings.Cut(rest, `"`)
	return b
}

func TestTheEnvelopeSenderIsTheBareAddress(t *testing.T) {
	for in, want := range map[string]string{
		`"Shop, Inc" <hi@shop.test>`: "hi@shop.test",
		"hi@shop.test":               "hi@shop.test",
		"Shop <hi@shop.test>":        "hi@shop.test",
	} {
		if got := extractAddr(in); got != want {
			t.Errorf("extractAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFromHeader(t *testing.T) {
	for _, tc := range []struct{ name, addr, want string }{
		{"", "", FallbackFromAddress},
		{"  ", "hi@shop.test", "hi@shop.test"},
		{"Shop, Inc", "hi@shop.test", `"Shop, Inc" <hi@shop.test>`},
		{"Prodavnica Čačak", "hi@shop.test", "=?utf-8?q?Prodavnica_=C4=8Ca=C4=8Dak?= <hi@shop.test>"},
	} {
		if got := FromHeader(tc.name, tc.addr); got != tc.want {
			t.Errorf("FromHeader(%q, %q) = %q, want %q", tc.name, tc.addr, got, tc.want)
		}
	}
}
