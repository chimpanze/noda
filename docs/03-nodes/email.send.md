# email.send

Sends an email via SMTP.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `to` | string (expr) | yes | Recipient(s) |
| `subject` | string (expr) | yes | Subject line |
| `body` | string (expr) | yes | Email body |
| `from` | string (expr) | no | Sender (overrides service default) |
| `cc` | string (expr) | no | CC recipients |
| `bcc` | string (expr) | no | BCC recipients |
| `reply_to` | string (expr) | no | Reply-To address |
| `content_type` | string | no | `"html"` (default) or `"text"` |

## Outputs

`success`, `error`

Output: `{message_id}`

## Behavior

Resolves all expression fields. Sends the email through the configured SMTP service. Default `content_type` is `"html"`. Fires `success` with the message ID on delivery.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `mailer` | `email` | Yes |

## Example

```json
{
  "type": "email.send",
  "services": { "mailer": "smtp" },
  "config": {
    "to": "{{ input.email }}",
    "subject": "Welcome to our platform!",
    "body": "<h1>Welcome, {{ input.name }}!</h1>",
    "content_type": "html"
  }
}
```

### With data flow

After a user resets their password, send a confirmation email using data from the user lookup.

```json
{
  "send_confirmation": {
    "type": "email.send",
    "services": { "mailer": "smtp" },
    "config": {
      "to": "{{ nodes.find_user.email }}",
      "subject": "Password reset successful",
      "body": "<p>Hi {{ nodes.find_user.name }}, your password was reset at {{ nodes.reset_time }}.</p>",
      "content_type": "html"
    }
  }
}
```

When `nodes.find_user` produced `{"id": 42, "email": "alice@example.com", "name": "Alice"}` and `nodes.reset_time` produced `"2024-01-15T10:30:00Z"`, the email is sent to `alice@example.com`. Output stored as `nodes.send_confirmation`:
```json
{ "message_id": "abc-123-def" }
```
