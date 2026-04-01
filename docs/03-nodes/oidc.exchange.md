# oidc.exchange

Exchanges an authorization code for OIDC tokens.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `issuer_url` | string (expr) | yes | OIDC provider issuer URL |
| `client_id` | string (expr) | yes | OAuth2 client ID |
| `client_secret` | string (expr) | yes | OAuth2 client secret |
| `redirect_uri` | string (expr) | yes | Callback URL used during authorization |
| `code` | string (expr) | yes | Authorization code to exchange (typically `{{ query.code }}`) |

## Outputs

`success`, `error`

## Behavior

Performs OIDC discovery, exchanges the authorization code at the provider's token endpoint, and verifies the returned ID token using the provider's JWKS.

On success, outputs:

| Field | Description |
|-------|-------------|
| `id_token` | Raw ID token string |
| `access_token` | OAuth2 access token |
| `refresh_token` | Refresh token (if provided by the IdP) |
| `claims` | Decoded ID token claims as an object |
| `expires_at` | Token expiry as Unix timestamp |

## Example

```json
{
  "type": "oidc.exchange",
  "config": {
    "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
    "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
    "client_secret": "{{ $env('OIDC_CLIENT_SECRET') }}",
    "redirect_uri": "http://localhost:3000/auth/callback",
    "code": "{{ input.code }}"
  }
}
```

### With data flow

An OAuth callback endpoint exchanges the authorization code, then uses the returned claims to find or create a user record.

```json
{
  "exchange": {
    "type": "oidc.exchange",
    "config": {
      "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
      "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
      "client_secret": "{{ $env('OIDC_CLIENT_SECRET') }}",
      "redirect_uri": "{{ $env('APP_URL') + '/auth/callback' }}",
      "code": "{{ query.code }}"
    }
  },
  "upsert_user": {
    "type": "db.upsert",
    "services": { "database": "postgres" },
    "config": {
      "table": "users",
      "conflict": ["oidc_sub"],
      "data": {
        "oidc_sub": "{{ nodes.exchange.claims.sub }}",
        "email": "{{ nodes.exchange.claims.email }}",
        "name": "{{ nodes.exchange.claims.name }}"
      }
    }
  }
}
```

Output stored as `nodes.exchange`:
```json
{
  "id_token": "eyJ...",
  "access_token": "ya29...",
  "refresh_token": "1//0e...",
  "claims": { "sub": "10269", "email": "user@example.com", "name": "Jane" },
  "expires_at": 1717200000
}
```

Downstream nodes access identity fields via `nodes.exchange.claims.email` or `nodes.exchange.access_token`.
