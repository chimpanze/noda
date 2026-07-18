# Cookbook: email nodes

Runnable example for `email.send`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

```bash
noda start --config examples/node-cookbook/email
```

## email.send — `POST /api/send`

Sends an email via SMTP. The harness provisions Mailpit for this example and verifies that the email lands in the real inbox.

```bash
curl -X POST localhost:3000/api/send -H 'Content-Type: application/json' -d '{"to": "bob@example.com", "subject": "Cookbook hello", "body": "greetings from the cookbook"}'
# → 202 {"sent":true}
```

The test harness (`verify.json`) asserts that the message lands in the Mailpit inbox with the correct recipient, subject, and body regex.
