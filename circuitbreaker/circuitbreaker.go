package circuitbreaker

import (
	"errors"
	"log"
	"net/http"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type Config struct {
	FailureThreshold int
	Timeout          time.Duration
	HalfOpenMax      int
}

type Option func(*Config)

func FailureThreshold(n int) Option {
	return func(c *Config) {
		c.FailureThreshold = n
	}
}

func Timeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

func defaultConfig() *Config {
	return &Config{
		FailureThreshold: 5,
		Timeout:          30 * time.Second,
		HalfOpenMax:      3,
	}
}

type circuitBreaker struct {
	mu               sync.Mutex
	state            State
	failures         int
	halfOpenAttempts int
	lastFailTime     time.Time
	config           *Config
}

func (cb *circuitBreaker) call(fn func() (*http.Response, error)) (*http.Response, error) {
	cb.mu.Lock()

	if cb.state == StateOpen {
		if time.Since(cb.lastFailTime) > cb.config.Timeout {
			log.Printf("[CIRCUIT] transitioning to half-open")
			cb.state = StateHalfOpen
			cb.halfOpenAttempts = 0
		} else {
			cb.mu.Unlock()
			return nil, ErrCircuitOpen
		}
	}

	if cb.state == StateHalfOpen && cb.halfOpenAttempts >= cb.config.HalfOpenMax {
		cb.mu.Unlock()
		return nil, ErrCircuitOpen
	}

	if cb.state == StateHalfOpen {
		cb.halfOpenAttempts++
	}

	cb.mu.Unlock()

	resp, err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil || resp.StatusCode >= http.StatusInternalServerError {
		cb.failures++
		cb.lastFailTime = time.Now()

		if cb.state == StateHalfOpen {
			log.Printf("[CIRCUIT] half-open test failed, reopening circuit")
			cb.state = StateOpen
			cb.halfOpenAttempts = 0
		} else if cb.failures >= cb.config.FailureThreshold {
			log.Printf("[CIRCUIT] failure threshold reached (%d), opening circuit", cb.failures)
			cb.state = StateOpen
		}

		return resp, err
	}

	if cb.state == StateHalfOpen {
		log.Printf("[CIRCUIT] half-open test succeeded, closing circuit")
		cb.state = StateClosed
		cb.failures = 0
		cb.halfOpenAttempts = 0
	} else if cb.state == StateClosed {
		cb.failures = 0
	}

	return resp, err
}

type cbTransport struct {
	inner http.RoundTripper
	cb    *circuitBreaker
}

func (t *cbTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.cb.call(func() (*http.Response, error) {
		return t.inner.RoundTrip(req)
	})
}

func Wrap(client *http.Client, opts ...Option) *http.Client {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	inner := client.Transport
	if inner == nil {
		inner = http.DefaultTransport
	}

	clone := *client
	clone.Transport = &cbTransport{
		inner: inner,
		cb: &circuitBreaker{
			state:  StateClosed,
			config: cfg,
		},
	}
	return &clone
}
