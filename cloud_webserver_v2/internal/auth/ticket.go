package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

// TicketExchange makes the one-time CAS ticket exchange idempotent.
//
// A CAS service ticket can only be exchanged once: the CAS server drops it from its
// registry as soon as it is validated, so any further exchange is answered with
// INVALID_TICKET. Browsers, however, do not guarantee that a callback URL is visited
// exactly once. A page with preloading enabled can fetch the redirect target before the
// real navigation, the user can reload the callback, and a stalled redirect can be
// retried. Each of those sends the same ticket to our callback a second time, which used
// to be reported as a failed login even though the first request had already succeeded.
//
// TicketExchange collapses concurrent exchanges of the same ticket into a single CAS
// request and replays the outcome to later requests for a short window, so a duplicated
// callback returns the same result instead of consuming the ticket twice.
type TicketExchange struct {
	exchange func(ctx context.Context, ticket string) (*User, error)
	window   time.Duration

	mu      sync.Mutex
	results map[string]*ticketExchangeResult
}

type ticketExchangeResult struct {
	done      chan struct{}
	user      *User
	err       error
	expiresAt time.Time
}

// NewTicketExchange wraps a single-use ticket exchange. The window bounds how long the
// outcome of an exchange is replayed, so it should stay close to the lifetime of a CAS
// service ticket.
func NewTicketExchange(window time.Duration, exchange func(ctx context.Context, ticket string) (*User, error)) *TicketExchange {
	return &TicketExchange{
		exchange: exchange,
		window:   window,
		results:  make(map[string]*ticketExchangeResult),
	}
}

// Exchange returns the user the ticket belongs to. The first caller performs the CAS
// request, callers that arrive while it is in flight wait for it, and callers that arrive
// within the replay window receive the same outcome without touching CAS again.
func (e *TicketExchange) Exchange(ctx context.Context, ticket string) (*User, error) {
	if ticket == "" {
		// Nothing to deduplicate; let the exchange report the missing ticket.
		return e.exchange(ctx, ticket)
	}

	e.mu.Lock()
	e.forgetExpiredLocked()

	if result, ok := e.results[ticket]; ok {
		e.mu.Unlock()
		return result.wait(ctx)
	}

	result := &ticketExchangeResult{
		done:      make(chan struct{}),
		expiresAt: time.Now().Add(e.window),
	}
	e.results[ticket] = result
	e.mu.Unlock()

	result.user, result.err = e.exchange(ctx, ticket)
	close(result.done)

	// Only a definitive answer from CAS is worth replaying. Transport, context and parse
	// errors may be transient, so drop them and let the next request try again.
	if !definitiveResult(result.err) {
		e.mu.Lock()
		delete(e.results, ticket)
		e.mu.Unlock()
	}

	return result.user, result.err
}

func (r *ticketExchangeResult) wait(ctx context.Context) (*User, error) {
	select {
	case <-r.done:
		return r.user, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// definitiveResult reports whether an exchange outcome would be identical if the exchange
// were repeated, that is whether it came from CAS itself: a successful login or an
// authenticationFailure it returned.
func definitiveResult(err error) bool {
	if err == nil {
		return true
	}

	var validation *ValidationError
	return errors.As(err, &validation)
}

func (e *TicketExchange) forgetExpiredLocked() {
	now := time.Now()
	for ticket, result := range e.results {
		if now.After(result.expiresAt) {
			delete(e.results, ticket)
		}
	}
}
