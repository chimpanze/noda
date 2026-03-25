package metrics

import (
	"net/http"

	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config holds configuration for the metrics subsystem.
type Config struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"` // default "/metrics"
}

// Metrics holds all application-level metric instruments.
type Metrics struct {
	// HTTP
	RequestDuration metric.Float64Histogram
	RequestsTotal   metric.Int64Counter
	ErrorsTotal     metric.Int64Counter

	// Workflows
	WorkflowDuration metric.Float64Histogram
	WorkflowsTotal   metric.Int64Counter
	WorkflowErrors   metric.Int64Counter

	// Nodes
	NodeDuration metric.Float64Histogram
	NodeErrors   metric.Int64Counter

	// Connections
	ActiveConns metric.Int64UpDownCounter

	// Panics
	PanicsRecovered metric.Int64Counter
}

// NewMetrics creates all metric instruments on the given meter.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	m := &Metrics{}
	var err error

	// HTTP metrics
	m.RequestDuration, err = meter.Float64Histogram("http.request.duration",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	m.RequestsTotal, err = meter.Int64Counter("http.requests.total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return nil, err
	}

	m.ErrorsTotal, err = meter.Int64Counter("http.errors.total",
		metric.WithDescription("Total number of HTTP errors"),
	)
	if err != nil {
		return nil, err
	}

	// Workflow metrics
	m.WorkflowDuration, err = meter.Float64Histogram("workflow.duration",
		metric.WithDescription("Duration of workflow executions in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	m.WorkflowsTotal, err = meter.Int64Counter("workflow.executions.total",
		metric.WithDescription("Total number of workflow executions"),
	)
	if err != nil {
		return nil, err
	}

	m.WorkflowErrors, err = meter.Int64Counter("workflow.errors.total",
		metric.WithDescription("Total number of failed workflow executions"),
	)
	if err != nil {
		return nil, err
	}

	// Node metrics
	m.NodeDuration, err = meter.Float64Histogram("node.duration",
		metric.WithDescription("Duration of node executions in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	m.NodeErrors, err = meter.Int64Counter("node.errors.total",
		metric.WithDescription("Total number of node execution errors"),
	)
	if err != nil {
		return nil, err
	}

	// Connection metrics
	m.ActiveConns, err = meter.Int64UpDownCounter("connections.active",
		metric.WithDescription("Number of active WebSocket/SSE connections"),
	)
	if err != nil {
		return nil, err
	}

	// Panic metrics
	m.PanicsRecovered, err = meter.Int64Counter("panics.recovered.total",
		metric.WithDescription("Total number of recovered panics"),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// NewProvider creates an OTel meter provider with a Prometheus exporter.
// It returns the provider and an http.Handler that serves the /metrics endpoint.
func NewProvider() (*sdkmetric.MeterProvider, http.Handler, error) {
	registry := promclient.NewRegistry()
	exporter, err := prometheus.New(
		prometheus.WithRegisterer(registry),
	)
	if err != nil {
		return nil, nil, err
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return provider, handler, nil
}

// ParseConfig extracts metrics configuration from the root config map.
// It looks for root["observability"]["metrics"].
func ParseConfig(root map[string]any) Config {
	cfg := Config{
		Path: "/metrics",
	}

	obs, ok := root["observability"].(map[string]any)
	if !ok {
		return cfg
	}

	mc, ok := obs["metrics"].(map[string]any)
	if !ok {
		return cfg
	}

	if enabled, ok := mc["enabled"].(bool); ok {
		cfg.Enabled = enabled
	}

	if path, ok := mc["path"].(string); ok && path != "" {
		cfg.Path = path
	}

	return cfg
}
