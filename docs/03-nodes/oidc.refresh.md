# oidc.refresh

Refreshes OIDC tokens using a refresh token.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `issuer_url` | string (expr) | yes | OIDC provider issuer URL |
| `client_id` | string (expr) | yes | OAuth2 client ID |
| `client_secret` | string (expr) | yes | OAuth2 client secret |
| `refresh_token` | string (expr) | yes | Refresh token to use |

## Outputs

`success`, `error`

## Behavior

Performs OIDC discovery and uses the provider's token endpoint to exchange a refresh token for new tokens. If the provider returns a new ID token, it is verified and its claims are extracted. If ID token verification or claims extraction fails, the node routes to the `error` output.

On success, outputs:

| Field | Description |
|-------|-------------|
| `access_token` | New OAuth2 access token |
| `refresh_token` | New refresh token (if rotated by the IdP) |
| `id_token` | New ID token string (if returned) |
| `claims` | Decoded ID token claims (if ID token returned) |
| `expires_at` | Token expiry as Unix timestamp |

## Example

```json
{
  "type": "oidc.refresh",
  "config": {
    "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
    "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
    "client_secret": "{{ $env('OIDC_CLIENT_SECRET') }}",
    "refresh_token": "{{ input.refresh_token }}"
  }
}
```

### With data flow

A token refresh endpoint reads the stored refresh token from the database, refreshes the tokens, then updates the record with the new values.

```json
{
  "get_session": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "sessions",
      "where": { "user_id": "{{ auth.user_id }}" },
      "required": true
    }
  },
  "refresh": {
    "type": "oidc.refresh",
    "config": {
      "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
      "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
      "client_secret": "{{ $env('OIDC_CLIENT_SECRET') }}",
      "refresh_token": "{{ nodes.get_session.refresh_token }}"
    }
  },
  "update_session": {
    "type": "db.update",
    "services": { "database": "postgres" },
    "config": {
      "table": "sessions",
      "where": { "user_id": "{{ auth.user_id }}" },
      "data": {
        "access_token": "{{ nodes.refresh.access_token }}",
        "refresh_token": "{{ nodes.refresh.refresh_token }}",
        "expires_at": "{{ nodes.refresh.expires_at }}"
      }
    }
  }
}
```

Output stored as `nodes.refresh`:
```json
{
  "access_token": "ya29...",
  "refresh_token": "1//0e...",
  "id_token": "eyJ...",
  "claims": { "sub": "10269", "email": "user@example.com" },
  "expires_at": 1717203600
}
```

Downstream nodes access the new tokens via `nodes.refresh.access_token`.
