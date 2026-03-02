package retry

import (
	"errors"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/FirasYousfi/httpclient/circuitbreaker"
)

var (
	retryOutcome = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "httpclient_retry_outcome_total",
		Help: "Final outcome of each logical request after all retry attempts.",
	}, []string{"service", "status_code", "retried"})

	retryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "httpclient_retry_duration_seconds",
		Help:    "Application-level request duration including retries and circuit breaker rejections.",
		Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	}, []string{"service"})
)

type Config struct {
	MaxAttempts     int
	RetryableStatus map[int]bool
	Backoff         func(attempt int) time.Duration
	ServiceName     string
}

type Option func(*Config)

func MaxAttempts(n int) Option {
	return func(c *Config) {
		c.MaxAttempts = n
	}
}

func RetryableCodes(codes ...int) Option {
	return func(c *Config) {
		c.RetryableStatus = make(map[int]bool)
		for _, code := range codes {
			c.RetryableStatus[code] = true
		}
	}
}

func WithServiceName(name string) Option {
	return func(c *Config) {
		c.ServiceName = name
	}
}

func defaultConfig() *Config {
	return &Config{
		MaxAttempts: 3,
		RetryableStatus: map[int]bool{
			500: true,
			502: true,
			503: true,
			504: true,
		},
		Backoff: exponentialBackoff,
	}
}

func exponentialBackoff(attempt int) time.Duration {
	base := 100 * time.Millisecond
	max := 2 * time.Second

	delay := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if delay > max {
		delay = max
	}

	jitter := time.Duration(rand.Int63n(int64(delay / 2)))
	return delay + jitter
}

type retryTransport struct {
	inner  http.RoundTripper
	config *Config
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	var resp *http.Response
	var err error

	for attempt := 0; attempt < t.config.MaxAttempts; attempt++ {
		if attempt > 0 {
			wait := t.config.Backoff(attempt - 1)
			log.Printf("[RETRY] attempt %d/%d after %v", attempt+1, t.config.MaxAttempts, wait)
			time.Sleep(wait)
		}

		resp, err = t.inner.RoundTrip(req)

		if err != nil {
			log.Printf("[RETRY] attempt %d failed: %v", attempt+1, err)

			if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
				log.Printf("[RETRY] circuit breaker is open, not retrying")
				t.recordOutcome("error", attempt > 0)
				t.recordDuration(start)
				return nil, err
			}

			continue
		}

		if !t.config.RetryableStatus[resp.StatusCode] {
			t.recordOutcome(strconv.Itoa(resp.StatusCode), attempt > 0)
			t.recordDuration(start)
			return resp, nil
		}

		log.Printf("[RETRY] attempt %d got retryable status %d", attempt+1, resp.StatusCode)

		if resp.Body != nil {
			resp.Body.Close()
		}
	}

	if resp != nil {
		t.recordOutcome(strconv.Itoa(resp.StatusCode), true)
	} else {
		t.recordOutcome("error", true)
	}
	t.recordDuration(start)

	return resp, err
}

func (t *retryTransport) recordOutcome(statusCode string, retried bool) {
	if t.config.ServiceName == "" {
		return
	}
	r := "false"
	if retried {
		r = "true"
	}
	retryOutcome.WithLabelValues(t.config.ServiceName, statusCode, r).Inc()
}

func (t *retryTransport) recordDuration(start time.Time) {
	if t.config.ServiceName == "" {
		return
	}
	retryDuration.WithLabelValues(t.config.ServiceName).Observe(time.Since(start).Seconds())
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
	clone.Transport = &retryTransport{
		inner:  inner,
		config: cfg,
	}
	return &clone
}
