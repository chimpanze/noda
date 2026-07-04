# auth.create_token

Mints a single-use token (email verification, password reset).

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string (expr) | yes | User id (expression) |
| `purpose` | string | yes | One of `"verify_email"`, `"reset_password"` |
| `ttl` | string | no | Lifetime (e.g. `"1h"`); defaults per purpose from the service config (`tokens.verify_email_ttl` / `tokens.reset_password_ttl`) |

## Outputs

`success`, `error`

`success` output:

| Field | Type | Description |
|-------|------|-------------|
| `token` | string | Raw token. It exists only in workflow state for this execution — send it via `email.send`; only its SHA-256 hash is persisted. |
| `expires_at` | string (timestamp) | Token expiry |

## Behavior

Invalidates any prior unconsumed token for the same `user_id` + `purpose` (marks it `consumed_at`) before minting a new one, so only the most recently issued token of a given purpose is ever valid. Generates a 256-bit random token, stores its hash in `auth_tokens` with the requested purpose and expiry, and returns the raw value for the caller to deliver (typically by email — the node never sends anything itself).

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.create_token",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "user_id": "{{ nodes.create_user.id }}", "purpose": "verify_email" }
}
```

### With data flow

`auth-register` (scaffolded by `noda auth init`) mints a verification token right after creating the user, then emails it:

```json
{
  "verify_token": {
    "type": "auth.create_token",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "user_id": "{{ nodes.create_user.id }}", "purpose": "verify_email" }
  },
  "send_verify_email": {
    "type": "email.send",
    "services": { "mailer": "email" },
    "config": {
      "to": "{{ nodes.create_user.email }}",
      "subject": "Verify your email",
      "body": "<p>Welcome! Verify your email with this token: <strong>{{ nodes.verify_token.token }}</strong></p>"
    }
  }
}
```

`auth-request-password-reset` uses `purpose: "reset_password"` the same way. Because the response body for that flow is identical whether or not the email exists (see the [authentication guide](../04-guides/authentication.md)), only the presence of this node's `create_token` → `email.send` chain — which never runs for an unknown email — differs observably, and only in timing. Decoupling the email send with `event.emit` closes that gap; see the guide for the pattern.
