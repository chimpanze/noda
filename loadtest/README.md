# Load Testing

HTTP load tests for Noda using [k6](https://k6.io).

## Prerequisites

Install k6: https://grafana.com/docs/k6/latest/set-up/install-k6/

```bash
# macOS
brew install k6
```

## Quick Start

```bash
# Start Noda (production mode, minimal logging)
LOG_LEVEL=warn noda start --config loadtest/project

# In another terminal, run the baseline test
make loadtest-baseline
```

## Scenarios

| Scenario | File | What it tests | VUs | Duration |
|----------|------|---------------|-----|----------|
| Baseline | `baseline.js` | GET `/health` — raw framework overhead | 50 | 30s |
| Workflow chain | `workflow-chain.js` | POST `/echo` — 5-node workflow (transform → transform → if → response) | 50 | 30s |
| Concurrent | `concurrent.js` | POST `/echo` — ramp from 10 to 500 VUs | 10→500 | 5m |

## Running

```bash
# All scenarios sequentially
make loadtest

# Individual scenarios
make loadtest-baseline
k6 run --env BASE_URL=http://localhost:3000 loadtest/scenarios/workflow-chain.js
k6 run --env BASE_URL=http://localhost:3000 loadtest/scenarios/concurrent.js

# Against a different host
make loadtest LOADTEST_BASE_URL=http://staging.example.com:3000
```

## Comparing Frameworks

To compare Noda against another framework, start the other framework on the same port and run the same scenario:

```bash
# 1. Run against Noda
LOG_LEVEL=warn noda start --config loadtest/project
k6 run --env BASE_URL=http://localhost:3000 loadtest/scenarios/concurrent.js

# 2. Stop Noda, start the other framework on :3000
#    (must serve POST /echo accepting {"name": "..."} and returning JSON)

# 3. Run the same scenario
k6 run --env BASE_URL=http://localhost:3000 loadtest/scenarios/concurrent.js
```

The route contract for `/echo`:
- **Method:** POST
- **Request body:** `{"name": "Alice"}`
- **Expected response:** 200 with a JSON body containing a `greeting` field

## Test Project

The `project/` directory contains a minimal Noda config used by the load tests:

```
project/
  noda.json              — server config (port 3000)
  routes/
    health.json          — GET /health → health-check workflow
    echo.json            — POST /echo → echo-chain workflow
  workflows/
    health-check.json    — single response.json node
    echo-chain.json      — 5-node chain: parse → enrich → if → ok/error response
```

## Tips

- Always use `LOG_LEVEL=warn` or `LOG_LEVEL=error` for production-mode benchmarks. Per-request logging can reduce throughput by ~50%.
- Close other applications to reduce noise.
- Run each scenario 2-3 times and compare medians for stable results.
- The `concurrent.js` scenario is the most realistic — it ramps load gradually and shows how latency degrades under pressure.
