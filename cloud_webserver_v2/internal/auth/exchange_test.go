package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCodeExchangeReplaysDefinitiveResult(t *testing.T) {
	exchange := NewCodeExchange(time.Minute)
	var calls int32

	first, err := exchange.Exchange(context.Background(), "code", func(context.Context) (*User, error) {
		atomic.AddInt32(&calls, 1)
		return &User{Username: "gburdell3"}, nil
	})
	if err != nil {
		t.Fatalf("Exchange() returned unexpected error: %v", err)
	}

	second, err := exchange.Exchange(context.Background(), "code", func(context.Context) (*User, error) {
		atomic.AddInt32(&calls, 1)
		return &User{Username: "someone-else"}, nil
	})
	if err != nil {
		t.Fatalf("second Exchange() returned unexpected error: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("exchange called %d times, want 1", got)
	}
	if second.Username != first.Username {
		t.Errorf("replayed user = %q, want %q", second.Username, first.Username)
	}
}

func TestCodeExchangeCollapsesConcurrentExchanges(t *testing.T) {
	exchange := NewCodeExchange(time.Minute)
	var calls int32
	release := make(chan struct{})

	var wg sync.WaitGroup
	results := make([]*User, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = exchange.Exchange(context.Background(), "code", func(context.Context) (*User, error) {
				atomic.AddInt32(&calls, 1)
				<-release
				return &User{Username: "gburdell3"}, nil
			})
		}(i)
	}

	// Give the first goroutine time to register the in-flight exchange.
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("exchange called %d times, want 1", got)
	}
	for i, err := range errs {
		if err != nil {
			t.Errorf("Exchange() #%d error = %v", i, err)
		}
		if results[i] == nil || results[i].Username != "gburdell3" {
			t.Errorf("Exchange() #%d user = %v, want gburdell3", i, results[i])
		}
	}
}

func TestCodeExchangeRetriesTransientErrors(t *testing.T) {
	exchange := NewCodeExchange(time.Minute)
	var calls int32
	transient := errors.New("connection reset")

	if _, err := exchange.Exchange(context.Background(), "code", func(context.Context) (*User, error) {
		atomic.AddInt32(&calls, 1)
		return nil, transient
	}); !errors.Is(err, transient) {
		t.Fatalf("Exchange() error = %v, want %v", err, transient)
	}

	user, err := exchange.Exchange(context.Background(), "code", func(context.Context) (*User, error) {
		atomic.AddInt32(&calls, 1)
		return &User{Username: "gburdell3"}, nil
	})
	if err != nil {
		t.Fatalf("second Exchange() returned unexpected error: %v", err)
	}
	if user.Username != "gburdell3" {
		t.Errorf("Username = %q, want gburdell3", user.Username)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("exchange called %d times, want 2", got)
	}
}

func TestCodeExchangeDoesNotReplayProviderRejectionsForever(t *testing.T) {
	exchange := NewCodeExchange(time.Nanosecond)
	var calls int32
	rejection := &ValidationError{Code: "invalid_grant"}

	if _, err := exchange.Exchange(context.Background(), "code", func(context.Context) (*User, error) {
		atomic.AddInt32(&calls, 1)
		return nil, rejection
	}); !errors.As(err, &rejection) {
		t.Fatalf("Exchange() error = %v, want a ValidationError", err)
	}

	// The replay window has elapsed, so the code is exchanged again.
	time.Sleep(time.Millisecond)
	if _, err := exchange.Exchange(context.Background(), "code", func(context.Context) (*User, error) {
		atomic.AddInt32(&calls, 1)
		return nil, rejection
	}); !errors.As(err, &rejection) {
		t.Fatalf("second Exchange() error = %v, want a ValidationError", err)
	}

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("exchange called %d times, want 2", got)
	}
}
