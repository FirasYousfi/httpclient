package httpclient

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/FirasYousfi/httpclient/circuitbreaker"
	"github.com/FirasYousfi/httpclient/logging"
	"github.com/FirasYousfi/httpclient/monitor"
	"github.com/FirasYousfi/httpclient/ratelimit"
	"github.com/FirasYousfi/httpclient/retry"
)

type Config struct {
	baseClient *http.Client

	// Rate limiting
	enableRateLimit bool
	rps             float64
	burst           int

	// Circuit breaker
	enableCircuitBreaker bool
	failureThreshold     int
	cbTimeout            time.Duration

	// Retry
	enableRetry    bool
	maxAttempts    int
	retryableCodes []int

	// Monitoring
	enableMonitoring bool
	serviceName      string

	// Logging
	enableLogging bool
	loggingOpts   []logging.Option
}

type Option func(*Config)

func WithRateLimiting(rps float64, burst int) Option {
	return func(c *Config) {
		c.enableRateLimit = true
		c.rps = rps
		c.burst = burst
	}
}

func WithCircuitBreaker(threshold int, timeout time.Duration) Option {
	return func(c *Config) {
		c.enableCircuitBreaker = true
		c.failureThreshold = threshold
		c.cbTimeout = timeout
	}
}

func WithRetries(maxAttempts int, retryableCodes ...int) Option {
	return func(c *Config) {
		c.enableRetry = true
		c.maxAttempts = maxAttempts
		c.retryableCodes = retryableCodes
	}
}

func WithMonitoring(serviceName string) Option {
	return func(c *Config) {
		c.enableMonitoring = true
		c.serviceName = serviceName
	}
}

func WithBaseClient(client *http.Client) Option {
	return func(c *Config) {
		c.baseClient = client
	}
}

func WithLogging(logger *slog.Logger, opts ...logging.Option) Option {
	return func(c *Config) {
		c.enableLogging = true
		c.loggingOpts = append([]logging.Option{logging.WithLogger(logger)}, opts...)
	}
}

// New creates an HTTP client with middleware applied inside-out: the last one wrapped is the first one called.
// Call order: retry → circuitbreaker → ratelimit → monitor → logging → http request
func New(opts ...Option) *http.Client {
	cfg := &Config{
		baseClient: &http.Client{},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	client := cfg.baseClient

	if cfg.enableLogging {
		client = logging.Wrap(client, cfg.loggingOpts...)
	}

	if cfg.enableMonitoring {
		client = monitor.Wrap(client, cfg.serviceName)
	}

	if cfg.enableRateLimit {
		rlOpts := []ratelimit.Option{
			ratelimit.RequestsPerSecond(cfg.rps),
			ratelimit.Burst(cfg.burst),
		}
		client = ratelimit.Wrap(client, rlOpts...)
	}

	if cfg.enableCircuitBreaker {
		cbOpts := []circuitbreaker.Option{
			circuitbreaker.FailureThreshold(cfg.failureThreshold),
			circuitbreaker.Timeout(cfg.cbTimeout),
		}
		client = circuitbreaker.Wrap(client, cbOpts...)
	}

	if cfg.enableRetry {
		retryOpts := []retry.Option{
			retry.MaxAttempts(cfg.maxAttempts),
		}
		if len(cfg.retryableCodes) > 0 {
			retryOpts = append(retryOpts, retry.RetryableCodes(cfg.retryableCodes...))
		}
		if cfg.enableMonitoring {
			retryOpts = append(retryOpts, retry.WithServiceName(cfg.serviceName))
		}
		client = retry.Wrap(client, retryOpts...)
	}

	return client
}
