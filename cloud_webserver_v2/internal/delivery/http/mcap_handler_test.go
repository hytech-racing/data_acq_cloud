package http

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Calls applyRateLimit from many goroutines at once
func TestApplyRateLimitConcurrent(t *testing.T) {
	h := &mcapHandler{lastResetTime: time.Now()}

	// Every call gets counted under the limit
	const requests = 20
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.applyRateLimit(httptest.NewRecorder())
		}()
	}

	// Waits for every goroutine to finish
	wg.Wait()

	if h.mcapFetchCount != requests {
		t.Errorf("expected %d fetches counted, got %d", requests, h.mcapFetchCount)
	}
}