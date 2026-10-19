package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CodeExchange makes the one-time authorization code exchange idempotent.
//
// An authorization code can only be redeemed once: the provider drops it as soon as it
// is exchanged, so any further exchange is answered with invalid_grant. Browsers,
// however, do not guarantee that a callback URL is visited exactly once. A page with
// preloading enabled can fetch the redirect target before the real navigation, the user
// can reload the callback, and a stalled redirect can be retried. Each of those sends
// the same code to our callback a second time.
//
// CodeExchange collapses concurrent exchanges of the same code into a single provider
// request and replays the outcome to later requests for a short window, so a duplicated
// callback returns the same result instead of failing on an already-used code.
type CodeExchange struct {
	window time.Duration
	mu      sync.Mutex
	results map[string]*codeExchangeResult
}

type codeExchangeResult struct {
	done      chan struct{}
	user      *User
	err       error
	expiresAt time.Time
}

// NewCodeExchange returns a deduplicator that replays an outcome for the given window.
// The window should stay close to the lifetime of an authorization code.
func NewCodeExchange(window time.Duration) *CodeExchange {
	return &CodeExchange{
		window:  window,
		results: make(map[string]*codeExchangeResult),
	}
}

// Exchange returns the user the code belongs to. The first caller runs exchange,
// callers that arrive while it is in flight wait for it, and callers that arrive within
// the replay window receive the same outcome without redeeming the code again.
func (e *CodeExchange) Exchange(ctx context.Context, code string, exchange func(context.Context) (*User, error)) (*User, error) {
	if code == "" {
		// Nothing to deduplicate; let the exchange report the missing code.
		return exchange(ctx)
	}

	e.mu.Lock()
	e.forgetExpiredLocked()

	if result, ok := e.results[code]; ok {
		e.mu.Unlock()
		return result.wait(ctx)
	}

	result := &codeExchangeResult{
		done:      make(chan struct{}),
		expiresAt: time.Now().Add(e.window),
	}
	e.results[code] = result
	e.mu.Unlock()

	result.user, result.err = exchange(ctx)
	close(result.done)

	// Only a definitive answer from the provider is worth replaying. Transport, context
	// and parse errors may be transient, so drop them and let the next request retry.
	if !definitiveResult(result.err) {
		e.mu.Lock()
		delete(e.results, code)
		e.mu.Unlock()
	}

	return result.user, result.err
}

func (r *codeExchangeResult) wait(ctx context.Context) (*User, error) {
	select {
	case <-r.done:
		return r.user, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// definitiveResult reports whether an exchange outcome would be identical if the
// exchange were repeated, that is whether it came from the provider itself: a successful
// login or an error it returned.
func definitiveResult(err error) bool {
	if err == nil {
		return true
	}

	var validation *ValidationError
	return errors.As(err, &validation)
}

func (e *CodeExchange) forgetExpiredLocked() {
	now := time.Now()
	for code, result := range e.results {
		if now.After(result.expiresAt) {
			delete(e.results, code)
		}
	}
}
