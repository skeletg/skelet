package signalsclient

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSegmentReadsFailFastOnAHungSuite(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		var held []net.Conn
		for {
			conn, err := ln.Accept()
			if err != nil {
				for _, c := range held {
					_ = c.Close()
				}
				return
			}
			held = append(held, conn)
		}
	}()

	c, err := New(ln.Addr().String(), "test-secret", time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetSegmentReadTimeout(100 * time.Millisecond)

	for i := range segmentTripThreshold {
		start := time.Now()
		_, err := c.CustomerSegments(context.Background(), uuid.New())
		if !Unreachable(err) {
			t.Fatalf("read %d: err = %v, want unreachable", i, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("read %d took %v, want the short segment deadline", i, d)
		}
	}
	start := time.Now()
	if _, err := c.CustomerSegments(context.Background(), uuid.New()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("read after the trip: err = %v, want ErrUnavailable", err)
	}
	if d := time.Since(start); d > 20*time.Millisecond {
		t.Fatalf("read after the trip took %v, want no call at all", d)
	}
}

func TestACallerThatGivesUpDoesNotTripTheSegmentBreaker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		var held []net.Conn
		for {
			conn, err := ln.Accept()
			if err != nil {
				for _, c := range held {
					_ = c.Close()
				}
				return
			}
			held = append(held, conn)
		}
	}()
	c, err := New(ln.Addr().String(), "test-secret", time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetSegmentReadTimeout(300 * time.Millisecond)

	for range segmentTripThreshold + 1 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, _ = c.CustomerSegments(ctx, uuid.New())
		cancel()
	}
	if _, err := c.CustomerSegments(context.Background(), uuid.New()); errors.Is(err, ErrUnavailable) {
		t.Fatal("the breaker tripped on callers' own deadlines, so segment reads were skipped with the suite healthy")
	}
}

// A probe whose caller gives up used to keep the half-open slot, so segment reads stayed off until a restart
func TestAnAbandonedProbeLetsTheNextReadThrough(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		var held []net.Conn
		for {
			conn, err := ln.Accept()
			if err != nil {
				for _, c := range held {
					_ = c.Close()
				}
				return
			}
			held = append(held, conn)
		}
	}()
	c, err := New(ln.Addr().String(), "test-secret", time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetSegmentReadTimeout(100 * time.Millisecond)
	now := time.Now()
	c.segments.breaker.SetNow(func() time.Time { return now })

	for range segmentTripThreshold {
		_, _ = c.CustomerSegments(context.Background(), uuid.New())
	}
	if _, err := c.CustomerSegments(context.Background(), uuid.New()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("read after the trip: err = %v, want ErrUnavailable", err)
	}
	now = now.Add(segmentCooldown + time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	_, _ = c.CustomerSegments(ctx, uuid.New())
	cancel()
	if _, err := c.CustomerSegments(context.Background(), uuid.New()); errors.Is(err, ErrUnavailable) {
		t.Fatal("the abandoned probe kept the circuit shut, want the next read to probe")
	}
}
