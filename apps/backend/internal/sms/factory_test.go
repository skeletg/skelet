package sms

import (
	"context"
	"strings"
	"testing"
)

// A blank env var is the same as an unset one - a half-filled .env is the usual way this goes wrong
func setTwilio(t *testing.T, sid, token, from string) {
	t.Helper()
	t.Setenv("TWILIO_ACCOUNT_SID", sid)
	t.Setenv("TWILIO_AUTH_TOKEN", token)
	t.Setenv("TWILIO_FROM", from)
}

func TestNewPicksTwilioOnlyWithEveryCredential(t *testing.T) {
	setTwilio(t, "AC123", "tok", "+15550000000")
	got, ok := New("production").(TwilioSender)
	if !ok {
		t.Fatalf("New = %T, want TwilioSender when all three vars are set", New("production"))
	}
	if got.AccountSID != "AC123" || got.AuthToken != "tok" || got.From != "+15550000000" {
		t.Errorf("sender = %+v, want the env values carried through", got)
	}
}

// A partially configured gateway must NOT silently become the dev logger in production - the whole
// point of errSender is that the misconfiguration is visible in the flow's failure log
func TestNewRefusesPartialCredentialsInProduction(t *testing.T) {
	for name, creds := range map[string][3]string{
		"no sid":     {"", "tok", "+1555"},
		"no token":   {"AC123", "", "+1555"},
		"no from":    {"AC123", "tok", ""},
		"nothing":    {"", "", ""},
		"only a sid": {"AC123", "", ""},
	} {
		setTwilio(t, creds[0], creds[1], creds[2])
		sender := New("production")
		if _, isTwilio := sender.(TwilioSender); isTwilio {
			t.Errorf("%s: New = TwilioSender, want it refused", name)
			continue
		}
		if _, isLog := sender.(LogSender); isLog {
			t.Errorf("%s: New = LogSender in production, want a loud errSender", name)
			continue
		}
		err := sender.Send(context.Background(), "+15551234567", "hi")
		if err == nil {
			t.Errorf("%s: Send = nil, want an error naming the misconfiguration", name)
			continue
		}
		if !strings.Contains(err.Error(), "not configured") {
			t.Errorf("%s: Send error = %v, want it to say the sender is not configured", name, err)
		}
	}
}

// Local dev has no gateway, and flows still have to be exercisable end to end
func TestNewFallsBackToTheLoggerInDevelopment(t *testing.T) {
	setTwilio(t, "", "", "")
	sender := New("development")
	if _, ok := sender.(LogSender); !ok {
		t.Fatalf("New(development) = %T, want LogSender", sender)
	}
	if err := sender.Send(context.Background(), "+15551234567", "hi"); err != nil {
		t.Errorf("LogSender.Send = %v, want nil so a flow never fails on a dev SMS step", err)
	}
}

// Real credentials win even in development, so a developer pointed at a Twilio test account gets it
func TestDevelopmentStillUsesTwilioWhenConfigured(t *testing.T) {
	setTwilio(t, "AC123", "tok", "+15550000000")
	if _, ok := New("development").(TwilioSender); !ok {
		t.Errorf("New(development) with credentials = %T, want TwilioSender", New("development"))
	}
}

// Anything that isn't exactly "development" is treated as production - a typo'd APP_ENV must fail
// closed rather than quietly logging real customer messages to stdout
func TestUnknownEnvironmentsFailClosed(t *testing.T) {
	setTwilio(t, "", "", "")
	for _, env := range []string{"", "dev", "Development", "staging", "prod"} {
		sender := New(env)
		if _, isLog := sender.(LogSender); isLog {
			t.Errorf("New(%q) = LogSender, want it treated as production", env)
		}
		if err := sender.Send(context.Background(), "+1555", "hi"); err == nil {
			t.Errorf("New(%q).Send = nil, want an error", env)
		}
	}
}
