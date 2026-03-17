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
