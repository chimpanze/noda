package trace

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracerConfig holds the configuration for OTel tracing.
type TracerConfig struct {
	Enabled  bool   `json:"enabled"`
	Exporter string `json:"exporter"` // "otlp", "stdout", or "" (noop)
	Endpoint string `json:"endpoint"` // OTLP endpoint
	Insecure bool   `json:"insecure"` // use HTTP instead of HTTPS
}

// Provider wraps the OTel tracer provider with shutdown support.
type Provider struct {
	provider *sdktrace.TracerProvider
	tracer   oteltrace.Tracer
	logger   *slog.Logger
}

// NewProvider creates a new OTel trace provider from config.
func NewProvider(ctx context.Context, cfg TracerConfig, logger *slog.Logger) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{
			tracer: noop.NewTracerProvider().Tracer("noda"),
			logger: logger,
		}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("noda"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	var opts []sdktrace.TracerProviderOption
	opts = append(opts, sdktrace.WithResource(res))

	switch cfg.Exporter {
	case "otlp":
		exportOpts := []otlptracehttp.Option{}
		if cfg.Endpoint != "" {
			exportOpts = append(exportOpts, otlptracehttp.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			exportOpts = append(exportOpts, otlptracehttp.WithInsecure())
		}
		exporter, err := otlptracehttp.New(ctx, exportOpts...)
		if err != nil {
			return nil, fmt.Errorf("create OTLP exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	default:
		// No exporter — just use the provider for in-process spans
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)

	logger.Info("otel tracing initialized", "exporter", cfg.Exporter)

	return &Provider{
		provider: tp,
		tracer:   tp.Tracer("noda"),
		logger:   logger,
	}, nil
}

// Tracer returns the OTel tracer.
func (p *Provider) Tracer() oteltrace.Tracer {
	return p.tracer
}

// Shutdown flushes and shuts down the tracer provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.provider == nil {
		return nil
	}
	p.logger.Info("flushing otel traces")
	return p.provider.Shutdown(ctx)
}

// StartWorkflowSpan creates a root span for a workflow execution.
func StartWorkflowSpan(ctx context.Context, tracer oteltrace.Tracer, workflowID, traceID, triggerType string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.Start(ctx, "workflow:"+workflowID,
		oteltrace.WithAttributes(
			attribute.String("workflow.id", workflowID),
			attribute.String("workflow.trace_id", traceID),
			attribute.String("workflow.trigger_type", triggerType),
		),
	)
	return ctx, span
}

// StartNodeSpan creates a child span for a node execution.
func StartNodeSpan(ctx context.Context, tracer oteltrace.Tracer, nodeID, nodeType string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.Start(ctx, "node:"+nodeType,
		oteltrace.WithAttributes(
			attribute.String("node.id", nodeID),
			attribute.String("node.type", nodeType),
		),
	)
	return ctx, span
}

// EndNodeSpan completes a node span with output and optional error.
func EndNodeSpan(span oteltrace.Span, output string, err error) {
	span.SetAttributes(attribute.String("node.output", output))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// EndWorkflowSpan completes a workflow span.
func EndWorkflowSpan(span oteltrace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// ParseConfig extracts tracing config from the root config map.
func ParseConfig(root map[string]any) TracerConfig {
	cfg := TracerConfig{}
	obs, ok := root["observability"].(map[string]any)
	if !ok {
		return cfg
	}
	tracing, ok := obs["tracing"].(map[string]any)
	if !ok {
		return cfg
	}
	if v, ok := tracing["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := tracing["exporter"].(string); ok {
		cfg.Exporter = v
	}
	if v, ok := tracing["endpoint"].(string); ok {
		cfg.Endpoint = v
	}
	if v, ok := tracing["insecure"].(bool); ok {
		cfg.Insecure = v
	}
	return cfg
}
