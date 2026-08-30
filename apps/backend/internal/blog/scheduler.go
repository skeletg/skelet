package blog

import (
	"context"
	"log"
	"time"
)

type Scheduler struct { // promotes scheduled posts whose publishedAt has passed - run as a background goroutine from main
	svc      *Service
	interval time.Duration
}

func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{svc: svc, interval: time.Minute}
}

func (s *Scheduler) SetInterval(d time.Duration) { // overrides the default 1-minute tick, for tests
	if d > 0 {
		s.interval = d
	}
}

func (s *Scheduler) Run(ctx context.Context) { // blocks until ctx is cancelled, ticking every interval
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.svc.PromoteDuePosts(ctx)
			if err != nil {
				log.Printf("[blog] scheduler: %v", err)
			} else if n > 0 {
				log.Printf("[blog] scheduler: promoted %d post(s) to published", n)
			}
		}
	}
}

func (s *Service) PromoteDuePosts(ctx context.Context) (int, error) { // flips every scheduled post whose publishedAt has passed to status=`published`, returning the count
	due, err := s.repo.ListDueScheduledPosts(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, p := range due {
		if _, err := s.repo.SetPostStatus(ctx, p.ID, PostStatusPublished, p.PublishedAt); err != nil {
			log.Printf("[blog] promote post %s: %v", p.ID, err)
			continue
		}
		count++
		if s.bus != nil {
			s.bus.Publish(ctx, &PostPublished{
				PostID:   p.ID,
				Slug:     p.Slug,
				AuthorID: p.AuthorID,
			})
		}
	}
	return count, nil
}
