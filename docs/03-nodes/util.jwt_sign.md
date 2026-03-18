# util.jwt_sign

Signs a JWT token with the given claims.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `claims` | object | yes | Claims to include in the token. Values are resolved as expressions. |
| `secret` | string | yes | Signing secret (expression). |
| `algorithm` | string | no | `"HS256"` (default), `"HS384"`, `"HS512"` |
| `expiry` | string | no | Token expiry duration (e.g. `"1h"`, `"24h"`, `"7d"`). Sets the `exp` claim. |

## Outputs

`success`, `error`

Output is the signed JWT token string.

## Behavior

Creates a JWT token with the specified claims, signs it using the given secret and algorithm, and fires `success` with the token string. Claim values that are strings are resolved as expressions. If `expiry` is set, an `exp` claim is added automatically.

## Example

```json
{
  "type": "util.jwt_sign",
  "config": {
    "claims": {
      "sub": "{{ input.user_id }}",
      "roles": "{{ input.roles }}"
    },
    "secret": "{{ secrets.JWT_SECRET }}",
    "algorithm": "HS256",
    "expiry": "24h"
  }
}
```
