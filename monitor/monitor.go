package monitor

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "httpclient_requests_total",
		Help: "Total number of HTTP requests made by the client.",
	}, []string{"service", "status_code"})

	errorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "httpclient_errors_total",
		Help: "Total number of HTTP request errors (transport-level).",
	}, []string{"service"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "httpclient_request_duration_seconds",
		Help:    "Histogram of HTTP request durations in seconds.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
	}, []string{"service"})
)

type instrumentedTransport struct {
	inner   http.RoundTripper
	service string
}

func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		errorsTotal.WithLabelValues(t.service).Inc()
		return nil, err
	}

	duration := time.Since(start).Seconds()
	code := strconv.Itoa(resp.StatusCode)
	requestsTotal.WithLabelValues(t.service, code).Inc()
	requestDuration.WithLabelValues(t.service).Observe(duration)

	return resp, nil
}

func Wrap(client *http.Client, service string) *http.Client {
	inner := client.Transport
	if inner == nil {
		inner = http.DefaultTransport
	}

	clone := *client
	clone.Transport = &instrumentedTransport{
		inner:   inner,
		service: service,
	}
	return &clone
}
