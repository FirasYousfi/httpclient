package circuitbreaker

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := Wrap(&http.Client{}, FailureThreshold(3))

	// Make 3 failing requests
	for i := 0; i < 3; i++ {
		client.Get(server.URL)
	}

	// 4th request should be rejected by circuit breaker
	_, err := client.Get(server.URL)

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected circuit breaker error, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts before circuit opened, got %d", attempts)
	}
}

func TestCircuitBreaker_RecoverAfterTimeout(t *testing.T) {
	failCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		if failCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := Wrap(&http.Client{},
		FailureThreshold(3),
		Timeout(100*time.Millisecond),
	)

	// Trigger circuit open
	for i := 0; i < 3; i++ {
		client.Get(server.URL)
	}

	// Circuit should be open
	_, err := client.Get(server.URL)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected circuit open, got %v", err)
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open and succeed
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
