package ratelimit

import "github.com/google/uuid"

// Fires when a client crosses from "slowed" into "refused" on an operator-facing throttle, meaning it kept hammering through seconds of backpressure, which no browser does
const EventThrottleTripped = "ratelimit.throttle_tripped"

type ThrottleTripped struct {
	IP        string
	Surface   string    // e.g. "catalog" - which throttled surface tripped
	Count     int64     // requests inside the current window when refusal began
	Budget    int       // the configured soft budget (req/min)
	ChannelID uuid.UUID // uuid.Nil when the request had no channel context
}

func (e *ThrottleTripped) Type() string { return EventThrottleTripped }
