package sms

import (
	"context"
	"fmt"
	"os"
)

// Picks a Sender, mirroring email.New - full Twilio credentials give TwilioSender, development without them gives LogSender, and anything else gives errSender so a prod misconfiguration surfaces loudly
// Never errors, since there's always some Sender - SMS sends are best-effort and the flow engine logs failures, so an errSender just makes the gap visible
func New(appEnv string) Sender {
	sid := os.Getenv("TWILIO_ACCOUNT_SID")
	token := os.Getenv("TWILIO_AUTH_TOKEN")
	from := os.Getenv("TWILIO_FROM")
	if sid != "" && token != "" && from != "" {
		return TwilioSender{AccountSID: sid, AuthToken: token, From: from}
	}
	if appEnv == "development" {
		return LogSender{}
	}
	return errSender{reason: "TWILIO_ACCOUNT_SID/AUTH_TOKEN/FROM not set and APP_ENV is not 'development'"}
}

type errSender struct{ reason string }

func (e errSender) Send(context.Context, string, string) error {
	return fmt.Errorf("sms sender not configured: %s", e.reason)
}
