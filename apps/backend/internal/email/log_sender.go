package email

import (
	"context"
	"log"
)

// Logs the message instead of sending - local dev only, since the log stream is the wrong trust boundary for tokens and you lose bounce and complaint signals
type LogSender struct{}

func (LogSender) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log.Printf("[email:log] to=%q from=%q subject=%q\n--- text ---\n%s\n--- end ---",
		msg.To, msg.From, msg.Subject, msg.TextBody)
	return nil
}
