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
