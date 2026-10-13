// Package email is a Sender abstraction for outbound mail - LogSender for dev and SMTPSender over net/smtp, chosen by env in factory.go
package email

import (
	"context"
	"net/mail"
	"strings"
)

type Message struct {
	From     string // "Display Name <address@domain>"
	To       string
	Subject  string
	HTMLBody string // transport formats the wire message (SMTP multipart/alternative)
	TextBody string
	// Extra header lines, written in key order - List-Unsubscribe and friends
	Headers map[string]string
}

// The RFC 2369 and RFC 8058 pair a bulk sender must carry - mailbox providers show their own unsubscribe button off them and POST the one-click form to the URL
func ListUnsubscribeHeaders(url string) map[string]string {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return nil
	}
	return map[string]string{
		"List-Unsubscribe":      "<" + url + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// The sender when no channel configured one - a dev default, which a real mail provider will refuse to send from
const FallbackFromAddress = "no-reply@skelet.local"

// One From header from a display name and an address - net/mail quotes or encodes the name as it needs, so "Shop, Inc" stays one mailbox and "Prodavnica Čačak" survives, where concatenating them read as two addresses
func FromHeader(name, addr string) string {
	name, addr = strings.TrimSpace(name), strings.TrimSpace(addr)
	if addr == "" {
		addr = FallbackFromAddress
	}
	if name == "" {
		return addr
	}
	return (&mail.Address{Name: name, Address: addr}).String()
}
