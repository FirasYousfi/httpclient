package ratelimit

import (
	"context"
	"net/http"

	"golang.org/x/time/rate"
)

type Config struct {
	RequestsPerSecond float64
	Burst             int
}

type Option func(*Config)

func RequestsPerSecond(rps float64) Option {
	return func(c *Config) {
		c.RequestsPerSecond = rps
	}
}

func Burst(b int) Option {
	return func(c *Config) {
		c.Burst = b
	}
}

func defaultConfig() *Config {
	return &Config{
		RequestsPerSecond: 10,
		Burst:             5,
	}
}

type rlTransport struct {
	inner   http.RoundTripper
	limiter *rate.Limiter
}

func (t *rlTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.limiter.Wait(context.Background())
	return t.inner.RoundTrip(req)
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

	limiter := rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst)

	clone := *client
	clone.Transport = &rlTransport{
		inner:   inner,
		limiter: limiter,
	}
	return &clone
}
