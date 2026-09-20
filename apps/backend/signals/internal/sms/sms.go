package sms

import "context"

type Sender interface {
	Send(ctx context.Context, to, body string) error
}
