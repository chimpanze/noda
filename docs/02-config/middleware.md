# Middleware

## Middleware Presets

Named collections of middleware for reuse across routes and route groups. Defined in `noda.json` under `middleware_presets`.

```json
{
  "middleware_presets": {
    "authenticated": ["auth.jwt"],
    "public": ["cors", "rate_limit"],
    "admin": ["auth.jwt", "auth.casbin"]
  }
}
```

Available middleware: `auth.jwt`, `auth.casbin`, `cors`, `rate_limit`, `helmet`, `compress`, `etag`, `livekit.webhook`.

## Route Groups

Apply middleware presets to URL path prefixes. Defined in `noda.json` under `route_groups`.

```json
{
  "route_groups": {
    "/api/admin": {
      "middleware_preset": "admin"
    },
    "/api": {
      "middleware_preset": "authenticated"
    }
  }
}
```

## livekit.webhook

Verifies LiveKit webhook signatures. Credentials are resolved from the middleware config first, then fall back to the `lk` service config in `noda.json`.

```json
{
  "path": "/webhooks/livekit",
  "method": "POST",
  "middleware": ["livekit.webhook"],
  "trigger": {
    "raw_body": true,
    "workflow": "on-livekit-event",
    "input": {
      "event": "{{ body.event }}",
      "room": "{{ body.room }}",
      "participant": "{{ body.participant }}"
    }
  }
}
```

The middleware only verifies the signature — the webhook body is accessed in your workflow through the normal `{{ body.* }}` trigger input mapping. LiveKit event types include: `room_started`, `room_finished`, `participant_joined`, `participant_left`, `track_published`, `track_unpublished`, `egress_started`, `egress_ended`, `ingress_started`, `ingress_ended`.

To provide credentials explicitly (instead of using the lk service config):

```json
{
  "security": {
    "livekit": {
      "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
      "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}"
    }
  }
}
```

## Route-Level Middleware

Individual routes can specify middleware directly:

```json
{
  "id": "update-task",
  "method": "PUT",
  "path": "/api/tasks/:id",
  "middleware": ["auth.jwt"]
}
```
