// Package email is a Sender abstraction for outbound mail - LogSender for dev and SMTPSender over net/smtp, chosen by env in factory.go
package email

import (
	"context"
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
