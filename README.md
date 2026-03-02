# httpclient

Go HTTP client with middleware: logging, monitoring, rate limiting, circuit breaker, retry.

Built on `http.RoundTripper` - wraps any `*http.Client` with composable middleware layers.

## Install

```bash
go get github.com/FirasYousfi/httpclient
```

## Usage

```go
client := httpclient.New(
    httpclient.WithRateLimiting(10, 20),              // 10 req/sec, burst of 20
    httpclient.WithCircuitBreaker(5, 30*time.Second), // open after 5 failures, test recovery after 30s
    httpclient.WithRetries(3),                        // 3 attempts, retries 500/502/503/504 by default
    httpclient.WithMonitoring("my-service"),          // Prometheus metrics
    httpclient.WithLogging(logger,                    // structured logging via slog
        logging.WithRequestHeaders(),
        logging.WithResponseHeaders(),
        logging.WithResponseBody(1024),
    ),
)

resp, err := client.Get("https://api.example.com/data")
```

### Wrap an existing client

```go
base := &http.Client{Timeout: 10 * time.Second}

client := httpclient.New(
    httpclient.WithBaseClient(base),
    httpclient.WithRetries(3),
    httpclient.WithMonitoring("my-service"),
)
```

### Use middlewares individually

Each middleware can be used standalone via its `Wrap()` function:

```go
import "github.com/FirasYousfi/httpclient/ratelimit"

client := &http.Client{}
client = ratelimit.Wrap(client, ratelimit.RequestsPerSecond(10), ratelimit.Burst(20))
```

## Middleware Order

Request flows: retry -> circuit breaker -> rate limit -> monitor -> logging -> HTTP

## Metrics

When monitoring is enabled, the following Prometheus metrics are exported:

- `httpclient_requests_total{service, status_code}` - per-attempt request count
- `httpclient_errors_total{service}` - transport errors
- `httpclient_request_duration_seconds{service}` - per-attempt latency
- `httpclient_retry_outcome_total{service, status_code, retried}` - final outcome after retries
- `httpclient_retry_duration_seconds{service}` - total latency including retries

## License

MIT
