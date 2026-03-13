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
