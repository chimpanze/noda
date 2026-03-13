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

Available middleware: `auth.jwt`, `auth.casbin`, `cors`, `rate_limit`, `helmet`, `compress`, `etag`.

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
