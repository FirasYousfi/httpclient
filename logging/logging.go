package logging

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Logger             *slog.Logger
	LogRequestHeaders  bool
	LogResponseHeaders bool
	LogRequestBody     bool
	LogResponseBody    bool
	MaxBodySize        int
}

type Option func(*Config)

func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

func WithRequestHeaders() Option {
	return func(c *Config) {
		c.LogRequestHeaders = true
	}
}

func WithResponseHeaders() Option {
	return func(c *Config) {
		c.LogResponseHeaders = true
	}
}

func WithRequestBody(maxSize int) Option {
	return func(c *Config) {
		c.LogRequestBody = true
		c.MaxBodySize = maxSize
	}
}

func WithResponseBody(maxSize int) Option {
	return func(c *Config) {
		c.LogResponseBody = true
		c.MaxBodySize = maxSize
	}
}

func defaultConfig() *Config {
	return &Config{
		Logger:      slog.Default(),
		MaxBodySize: 1024,
	}
}

var sensitiveHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"x-api-key":     true,
}

type loggingTransport struct {
	inner  http.RoundTripper
	config *Config
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	attrs := []slog.Attr{
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
	}

	if t.config.LogRequestHeaders {
		headers := make(map[string]string)
		for k, v := range req.Header {
			if !sensitiveHeaders[strings.ToLower(k)] {
				headers[k] = strings.Join(v, ", ")
			}
		}
		attrs = append(attrs, slog.Any("request_headers", headers))
	}

	if t.config.LogRequestBody && req.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(req.Body, int64(t.config.MaxBodySize)))
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
		attrs = append(attrs, slog.String("request_body", string(body)))
	}

	resp, err := t.inner.RoundTrip(req)

	duration := time.Since(start)
	attrs = append(attrs, slog.Duration("duration", duration))

	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		t.config.Logger.LogAttrs(req.Context(), slog.LevelError, "HTTP request failed", attrs...)
		return nil, err
	}

	attrs = append(attrs, slog.Int("status", resp.StatusCode))

	if t.config.LogResponseHeaders {
		headers := make(map[string]string)
		for k, v := range resp.Header {
			headers[k] = strings.Join(v, ", ")
		}
		attrs = append(attrs, slog.Any("response_headers", headers))
	}

	if t.config.LogResponseBody && resp.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(t.config.MaxBodySize)))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		attrs = append(attrs, slog.String("response_body", string(body)))
	}

	t.config.Logger.LogAttrs(req.Context(), slog.LevelInfo, "HTTP request completed", attrs...)

	return resp, nil
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
	clone.Transport = &loggingTransport{
		inner:  inner,
		config: cfg,
	}
	return &clone
}
