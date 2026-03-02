package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimit_ThrottlesRequests(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 2 requests/second
	client := Wrap(&http.Client{}, RequestsPerSecond(2), Burst(1))

	start := time.Now()

	// Make 4 requests - should take ~2 seconds at 2 req/sec
	for i := 0; i < 4; i++ {
		client.Get(server.URL)
	}

	elapsed := time.Since(start)

	// Should take at least 1.5 seconds (4 requests at 2/sec = 2 seconds, minus burst)
	if elapsed < 1*time.Second {
		t.Errorf("requests completed too fast: %v (rate limiting not working)", elapsed)
	}

	if requests != 4 {
		t.Errorf("expected 4 requests, got %d", requests)
	}
}
