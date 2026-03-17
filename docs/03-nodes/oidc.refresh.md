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

Performs OIDC discovery and uses the provider's token endpoint to exchange a refresh token for new tokens. If the provider returns a new ID token, it is verified and its claims are extracted.

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
