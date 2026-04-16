# Observability

Noda ships with OpenTelemetry instrumentation and a Prometheus exporter. Enable metrics and scrape the `/metrics` endpoint from your existing Prometheus stack.

## Enable metrics

Metrics are **off by default**. Opt in via `noda.json`:

```json
{
  "observability": {
    "metrics": {
      "enabled": true,
      "path": "/metrics"
    }
  }
}
```

- `enabled` — required, `true` to serve the endpoint
- `path` — optional, defaults to `/metrics`

The endpoint is served on the same port as your API (`server.port`, default `3000`). It is registered **before** global middleware, so CORS, rate limits, and auth do not apply — it is intended for scraping from a trusted network.

## Exposed metrics

| Metric | Type | Description |
|---|---|---|
| `http_request_duration_seconds` | histogram | HTTP request latency |
| `http_requests_total` | counter | Total HTTP requests |
| `http_errors_total` | counter | HTTP errors |
| `workflow_duration_seconds` | histogram | Workflow execution latency |
| `workflow_executions_total` | counter | Total workflow executions |
| `workflow_errors_total` | counter | Failed workflow executions |
| `node_duration_seconds` | histogram | Per-node execution latency |
| `node_errors_total` | counter | Node execution errors |
| `connections_active` | gauge | Active WebSocket / SSE connections |
| `panics_recovered_total` | counter | Recovered panics |

All metrics follow OpenTelemetry naming; the Prometheus exporter translates `.` to `_`.

## Scrape

```yaml
# prometheus.yml
scrape_configs:
  - job_name: noda
    static_configs:
      - targets: ["noda:3000"]
    metrics_path: /metrics
```

## Tracing

OpenTelemetry tracing is also enabled. Spans are emitted for HTTP requests, workflow executions, and each node execution. Configure an OTLP exporter via standard `OTEL_EXPORTER_*` environment variables.
