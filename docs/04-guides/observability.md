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

## Stream consumers

### Stream consumer auto-reclaim

`stream.Subscribe` automatically reclaims pending messages from crashed
consumer-group members. Every 10 read cycles, each consumer calls
`XAUTOCLAIM` for messages idle longer than 60 seconds and re-delivers
them through the same handler.

The 60-second threshold is shorter than the default per-message worker
handler timeout (5 minutes), so legitimate slow handlers don't get
their messages stolen mid-flight — but it's short enough that crashed
consumers don't tie up messages for hours.

Handlers MUST call `Ack` on success. Without an Ack, the message stays
pending and will be re-delivered to another consumer after 60 seconds
— at-least-once delivery semantics. End-to-end latency from a
consumer crash to another consumer re-attempting is approximately
`reclaimMinIdle` (60s) + `reclaimInterval × Block` (~20s) = **~80s**.

This is always-on — there is no config knob in v1. If you need a
different threshold, file an issue describing the use case.
