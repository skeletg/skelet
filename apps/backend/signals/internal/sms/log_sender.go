package sms

import (
	"context"
	"log"
)

// Logs the SMS instead of sending - the local-dev provider, so flows can be exercised without a real gateway
type LogSender struct{}

func (LogSender) Send(_ context.Context, to, body string) error {
	log.Printf("[sms] (log) to=%s body=%q", to, body)
	return nil
}
