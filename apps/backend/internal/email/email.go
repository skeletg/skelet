// Package email is a Sender abstraction for outbound mail - LogSender for dev and SMTPSender over net/smtp, chosen by env in factory.go
package email

import "context"

type Message struct {
	From     string // "Display Name <address@domain>"
	To       string
	Subject  string
	HTMLBody string // transport formats the wire message (SMTP multipart/alternative)
	TextBody string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}
