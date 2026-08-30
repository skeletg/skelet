package subscription

import (
	"context"
	"log"
	"time"
)

// Drives recurring billing - every interval it bills any subscription whose next charge is due - The interval is only the check frequency, since a sub bills only once its own next_billing_at has passed, so a short one is cheap
type Scheduler struct {
	svc      *Service
	interval time.Duration
}

func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{svc: svc, interval: time.Minute}
}

func (s *Scheduler) SetInterval(d time.Duration) {
	if d > 0 {
		s.interval = d
	}
}

// Blocks until ctx is cancelled, ticking every interval
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	// Bill once at startup so a fresh container converges without waiting
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	n, err := s.svc.ProcessDue(ctx, time.Now())
	if err != nil {
		log.Printf("[subscription] scheduler: %v", err)
	} else if n > 0 {
		log.Printf("[subscription] scheduler: billed %d subscription(s)", n)
	}
	// Collection runs after minting, so a just-due invoice can charge in the same tick
	c, err := s.svc.CollectDue(ctx, time.Now())
	if err != nil {
		log.Printf("[subscription] collector: %v", err)
	} else if c > 0 {
		log.Printf("[subscription] collector: collected %d invoice(s)", c)
	}
}
