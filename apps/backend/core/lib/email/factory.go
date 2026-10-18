package email

import (
	"context"
	"fmt"
	"os"
)

// Picks a Sender - SMTP_* set gives SMTPSender, dev without SMTP gives LogSender, and anything else gives errSender, which fails loudly in prod rather than silently dropping mail - Never errors
func New(appEnv string) Sender {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USERNAME")
	pass := os.Getenv("SMTP_PASSWORD")

	if host != "" && port != "" {
		return &SMTPSender{Host: host, Port: port, Username: user, Password: pass}
	}
	if appEnv == "development" {
		return LogSender{}
	}
	return errSender{reason: "SMTP_HOST/SMTP_PORT not set and APP_ENV is not 'development' - refusing to silently drop mail"}
}

type errSender struct{ reason string }

func (e errSender) Send(context.Context, Message) error {
	return fmt.Errorf("email sender not configured: %s", e.reason)
}
