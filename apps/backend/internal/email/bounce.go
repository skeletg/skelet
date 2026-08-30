package email

import (
	"regexp"
	"strings"
)

// Bounce classification for pure-SMTP deployments with no provider webhook - a permanent 5xx rejection at RCPT or DATA means the address is dead, so the send path can suppress it as a hard bounce
// A transient 4xx failure is retryable and must NOT suppress, and anything unclassifiable like a timeout or connection reset is treated as transient, since suppressing on an ambiguous network blip would wrongly block good addresses

var (
	basicCodeRe    = regexp.MustCompile(`\b([45])\d\d\b`)
	enhancedCodeRe = regexp.MustCompile(`\b([45])\.\d{1,3}\.\d{1,3}\b`)
)

// Inspects a Send error and reports whether it's a PERMANENT failure - matched is false when no SMTP code was found, and the caller should then treat it as transient
func ClassifyBounce(err error) (permanent, matched bool) {
	if err == nil {
		return false, false
	}
	msg := err.Error()
	if m := enhancedCodeRe.FindStringSubmatch(msg); m != nil {
		return m[1] == "5", true
	}
	if m := basicCodeRe.FindStringSubmatch(msg); m != nil {
		return m[1] == "5", true
	}
	lower := strings.ToLower(msg)
	for _, phrase := range []string{"user unknown", "no such user", "mailbox unavailable", "does not exist", "recipient rejected", "address rejected"} {
		if strings.Contains(lower, phrase) {
			return true, true
		}
	}
	return false, false
}
