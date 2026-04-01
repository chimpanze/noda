# oidc.auth_url

Builds an OIDC authorization URL for redirecting users to an identity provider.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `issuer_url` | string (expr) | yes | OIDC provider issuer URL |
| `client_id` | string (expr) | yes | OAuth2 client ID |
| `redirect_uri` | string (expr) | yes | Callback URL after authentication |
| `state` | string (expr) | yes | State parameter for CSRF protection |
| `scopes` | string[] | no | OAuth2 scopes (default: `["openid", "profile", "email"]`) |
| `extra_params` | object | no | Additional query parameters for the authorization URL |

## Outputs

`success`, `error`

## Behavior

Performs OIDC discovery on the `issuer_url`, then builds an OAuth2 authorization URL using the provider's authorization endpoint. The `state` parameter should be a random value stored in cache for later verification in the callback.

On success, outputs `{ url, state }`. Use `response.redirect` to send the user to the `url`.

## Example

```json
{
  "type": "oidc.auth_url",
  "config": {
    "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
    "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
    "redirect_uri": "http://localhost:3000/auth/callback",
    "state": "{{ $uuid() }}",
    "scopes": ["openid", "profile", "email"],
    "extra_params": {
      "prompt": "consent"
    }
  }
}
```

### With data flow

A login flow generates a state token, caches it for CSRF verification, then builds the authorization URL and redirects the user.

```json
{
  "gen_state": {
    "type": "transform.set",
    "config": {
      "state": "{{ $uuid() }}"
    }
  },
  "cache_state": {
    "type": "cache.set",
    "services": { "cache": "redis" },
    "config": {
      "key": "{{ 'oidc_state:' + nodes.gen_state.state }}",
      "value": "1",
      "ttl": 600
    }
  },
  "build_url": {
    "type": "oidc.auth_url",
    "config": {
      "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
      "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
      "redirect_uri": "{{ $env('APP_URL') + '/auth/callback' }}",
      "state": "{{ nodes.gen_state.state }}"
    }
  }
}
```

Output stored as `nodes.build_url`:
```json
{ "url": "https://accounts.google.com/o/oauth2/v2/auth?client_id=...&state=...", "state": "a1b2c3" }
```

Downstream nodes use `nodes.build_url.url` to redirect the user via `response.redirect`.
