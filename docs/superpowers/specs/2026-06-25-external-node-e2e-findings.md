# External-Service Node E2E — Findings

Bugs found and fixed while implementing `2026-06-25-external-node-e2e.md`.
One entry per bug: node, symptom, root cause, fix, guarding test.

## Bug 2 — `email.send` unconditionally forces implicit TLS, blocking plaintext SMTP servers

- **Node:** `email.send`
- **Symptom:** `email.send` could not connect to Mailpit's plaintext SMTP listener (port 1025) — the dial attempt used a TLS dialer, which Mailpit (and any other plaintext-only SMTP server) does not support.
- **Root cause:** `plugins/email/plugin.go`'s `CreateService` unconditionally defaulted `useTLS = true` regardless of port. Any service created without an explicit `"tls": false` config key would attempt an implicit TLS handshake (`tls.Dialer.DialContext`) against whatever port was configured — including non-TLS ports like 25, 587, and 1025.
- **Fix:** Two coordinated changes restore correct behaviour for all port classes:
  1. `plugins/email/plugin.go` — changed the default in `CreateService` to `useTLS = port == 465`. Port 465 is the conventional implicit-TLS (SMTPS) port; all other ports default to a plaintext TCP dial. An explicit `"tls"` config key still overrides the default in both directions.
  2. `plugins/email/service.go` — added opportunistic STARTTLS to the plaintext dial path in `dialCtx`. After `smtp.NewClient` succeeds on the raw connection, the code checks whether the server advertises the `STARTTLS` extension and, if so, upgrades the connection with `client.StartTLS`. If the upgrade fails the connection is closed and an error is returned; if the server does not advertise STARTTLS (e.g. Mailpit) the connection stays plaintext. The implicit-TLS branch (port 465, `tls.Dialer`) and the `dialFn` test-seam early-return are unchanged.
  ```go
  // plugin.go — Before:
  useTLS := true
  if v, ok := config["tls"].(bool); ok { useTLS = v }

  // plugin.go — After:
  useTLS := port == 465
  if v, ok := config["tls"].(bool); ok { useTLS = v }

  // service.go dialCtx — plaintext branch, After:
  conn, err := dialer.DialContext(ctx, "tcp", addr)
  if err != nil { return nil, err }
  client, err := smtp.NewClient(conn, s.host)
  if err != nil { return nil, err }
  if ok, _ := client.Extension("STARTTLS"); ok {
      if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
          _ = client.Close()
          return nil, err
      }
  }
  return client, nil
  ```
  Net behaviour: port 465 → implicit TLS (unchanged); port 587/25 with a STARTTLS-advertising server → plaintext connect then encrypted upgrade; Mailpit (no STARTTLS) → stays plaintext.
- **Guarding test:** `TestEmailSend_Engine` in `plugins/email/engine_e2e_integration_test.go` creates a service with only `host`, `port`, `from` (no `tls` key) and drives `email.send` against a live Mailpit container on its plaintext SMTP port. The test asserts delivery via the Mailpit HTTP API.

## Bug 1 — `db.create` does not return server-generated fields

- **Node:** `db.create`
- **Symptom:** The node returns `success` with the input `data` map unchanged; server-generated fields (e.g. `id` from a `serial PRIMARY KEY`, `created_at` defaults) are absent from the output even though the descriptor documents "The created row object including generated fields (id, created_at, etc.)".
- **Root cause:** `create.go` called `db.Table(table).Create(data)` without a `RETURNING` clause. GORM's `Create` with a `map[string]any` argument does not scan generated columns back into the map (unlike struct-based creates where GORM reads `LastInsertId` or a `Returning` clause automatically). The result was that the output map was identical to the input — `id` was nil.
- **Fix:** Added `clause.Returning{}` to the GORM call in `plugins/db/create.go`:
  ```go
  tx := db.WithContext(ctx).Table(table).Clauses(clause.Returning{}).Create(data)
  ```
  Postgres executes `INSERT … RETURNING *` and GORM populates the map with all returned columns, including `id`.
- **Guarding test:** `TestDBCreateAndFind_Engine` in `plugins/db/engine_e2e_integration_test.go` asserts `assert.NotNil(t, row["id"])` after a `db.create` call against a table with a `serial PRIMARY KEY`.

