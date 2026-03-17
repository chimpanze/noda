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
