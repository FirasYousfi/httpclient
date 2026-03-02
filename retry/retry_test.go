package retry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FirasYousfi/httpclient/circuitbreaker"
)

func TestRetry_Success(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := Wrap(&http.Client{}, MaxAttempts(3))
	resp, err := client.Get(server.URL)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetry_RetryOn500(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := Wrap(&http.Client{}, MaxAttempts(3))
	resp, err := client.Get(server.URL)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_DoesNotRetryCircuitBreakerError(t *testing.T) {
	attempts := 0

	// Create a transport that always returns circuit breaker error
	transport := &mockTransport{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			attempts++
			return nil, circuitbreaker.ErrCircuitOpen
		},
	}

	client := &http.Client{Transport: transport}
	client = Wrap(client, MaxAttempts(3))

	_, err := client.Get("http://example.com")

	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("expected circuit breaker error, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on circuit breaker), got %d", attempts)
	}
}

type mockTransport struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}
