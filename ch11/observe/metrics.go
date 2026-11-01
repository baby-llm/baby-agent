package observe

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics contains the low-cardinality service and LLM metrics exported at /metrics.
type Metrics struct {
	Requests     *prometheus.CounterVec
	InFlight     prometheus.Gauge
	Duration     *prometheus.HistogramVec
	FirstToken   *prometheus.HistogramVec
	Tokens       *prometheus.CounterVec
	TokensPerSec *prometheus.HistogramVec
	ToolCalls    *prometheus.CounterVec
	ToolDuration *prometheus.HistogramVec
}

func NewMetrics(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		Requests:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "babyagent_http_requests_total", Help: "HTTP requests by method, route and status."}, []string{"method", "route", "status"}),
		InFlight:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "babyagent_http_in_flight_requests", Help: "HTTP requests currently in flight."}),
		Duration:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "babyagent_http_request_duration_seconds", Help: "End-to-end HTTP request duration.", Buckets: []float64{.1, .5, 1, 3, 5, 10, 30, 60, 120, 300}}, []string{"method", "route", "status"}),
		FirstToken:   prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "babyagent_llm_first_token_duration_seconds", Help: "Time from LLM request to its first streamed token.", Buckets: []float64{.1, .5, 1, 3, 5, 10, 30}}, []string{"model"}),
		Tokens:       prometheus.NewCounterVec(prometheus.CounterOpts{Name: "babyagent_llm_tokens_total", Help: "LLM tokens consumed."}, []string{"model", "type"}),
		TokensPerSec: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "babyagent_llm_completion_tokens_per_second", Help: "Completion generation rate.", Buckets: []float64{1, 5, 10, 20, 40, 80, 160}}, []string{"model"}),
		ToolCalls:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "babyagent_tool_calls_total", Help: "Tool executions by outcome."}, []string{"tool", "status"}),
		ToolDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "babyagent_tool_duration_seconds", Help: "Tool execution duration.", Buckets: []float64{.01, .1, .5, 1, 3, 10, 30, 60}}, []string{"tool", "status"}),
	}
	registerer.MustRegister(m.Requests, m.InFlight, m.Duration, m.FirstToken, m.Tokens, m.TokensPerSec, m.ToolCalls, m.ToolDuration)
	return m
}

func (m *Metrics) ObserveLLM(model string, firstToken, streaming time.Duration, promptTokens, completionTokens int64) {
	if firstToken > 0 {
		m.FirstToken.WithLabelValues(model).Observe(firstToken.Seconds())
	}
	m.Tokens.WithLabelValues(model, "prompt").Add(float64(promptTokens))
	m.Tokens.WithLabelValues(model, "completion").Add(float64(completionTokens))
	if streaming > 0 && completionTokens > 0 {
		m.TokensPerSec.WithLabelValues(model).Observe(float64(completionTokens) / streaming.Seconds())
	}
}

func statusCode(code int) string { return strconv.Itoa(code) }
