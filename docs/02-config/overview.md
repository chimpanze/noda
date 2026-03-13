# Config Overview

This document covers every config file format in Noda with all fields, types, defaults, and examples.

## Config Directory Structure

```
project/
├── noda.json              # Root config (required)
├── vars.json              # Shared variables (optional)
├── routes/*.json          # HTTP route definitions
├── workflows/*.json       # Workflow DAGs
├── workers/*.json         # Event-driven worker subscriptions
├── schedules/*.json       # Cron job definitions
├── connections/*.json     # WebSocket and SSE endpoints
├── schemas/*.json         # JSON Schema definitions
├── tests/*.json           # Workflow test suites
├── migrations/*.sql       # SQL migration files
└── wasm/*.wasm            # Wasm modules
```

Noda discovers config files automatically from the config directory. Environment-specific overlays can be applied via `.env.json` or `--env` flag.

## Config Conventions

- **All field names** use `snake_case`
- **Duration values**: `"5s"`, `"100ms"`, `"1m"` (units: ms, s, m)
- **Size values**: `"10mb"`, `"64kb"`, `"1gb"` (units: kb, mb, gb)
- **Array fields** use plural names: `params`, `cases`, `fields`, `headers`, `cookies`
- **Static fields** (never expressions): `mode`, `cases`, `workflow`, `method`, `type`, `backoff`
- **Expression fields**: everything else that evaluates at runtime
