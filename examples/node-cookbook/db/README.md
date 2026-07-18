# Cookbook: db nodes

Runnable examples for `db.create`, `db.find`, `db.findOne`, `db.update`,
`db.delete`, `db.upsert`, `db.count`, `db.query`, and `db.exec` against a
Postgres-backed `books` table.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

This project needs a real Postgres instance — CI's cookbook walker starts one
via testcontainers automatically (see `deps: ["postgres"]` in `verify.json`).
To run it yourself:

```bash
docker run -d --name cookbook-db -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:17-alpine
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable'
go run ./cmd/noda migrate up --config examples/node-cookbook/db --service main-db
go run ./cmd/noda start --config examples/node-cookbook/db
```

## db.create — `POST /api/books`

Inserts a row and returns it, including the database-generated `id`.

```bash
curl -X POST localhost:3000/api/books -H 'Content-Type: application/json' \
  -d '{"title": "Dune", "author": "Herbert", "year": 1965}'
# → 201 {"id":"<uuid>","title":"Dune","author":"Herbert","year":1965,"created_at":"..."}
```

## db.find — `GET /api/books?author=X`

Structured SELECT with a `where` filter, ordered by `title ASC`.

```bash
curl localhost:3000/api/books?author=Herbert
# → 200 {"data":[{"id":"<uuid>","title":"Dune","author":"Herbert","year":1965,"created_at":"..."}]}
```

## db.findOne — `GET /api/books/:id`

Single-row SELECT. `required` defaults to `true`, so a miss fires the
`error` output (`NotFoundError`) — routed here to a 404 `response.error`.

```bash
curl localhost:3000/api/books/<id>
# → 200 {"id":"<uuid>","title":"Dune","author":"Herbert","year":1965,"created_at":"..."}
curl localhost:3000/api/books/00000000-0000-0000-0000-000000000000
# → 404 {"error":{"code":"NOT_FOUND","message":"Book not found"}}
```

## db.update — `PUT /api/books/:id`

`db.update`'s own output is `{"rows_affected": <count>}` only (per
`docs/03-nodes/db.update.md`) — it does not return the updated row. This
workflow follows the update with a `db.findOne` re-read so the response
carries the new `year`.

```bash
curl -X PUT localhost:3000/api/books/<id> -H 'Content-Type: application/json' \
  -d '{"year": 1966}'
# → 200 {"id":"<uuid>","title":"Dune","author":"Herbert","year":1966,"created_at":"..."}
```

## db.delete — `DELETE /api/books/:id`

```bash
curl -X DELETE localhost:3000/api/books/<id>
# → 204 (empty body)
```

## db.upsert — `POST /api/books/upsert`

Conflicts on `title` (see the `books_title_key` unique index added in the
migration) and updates `year` on conflict. Per
`docs/03-nodes/db.upsert.md`, the output is the resolved `data` map as sent —
no `RETURNING`, so no database-generated `id` comes back.

```bash
curl -X POST localhost:3000/api/books/upsert -H 'Content-Type: application/json' \
  -d '{"title": "Dune", "author": "Herbert", "year": 2021}'
# → 200 {"title":"Dune","author":"Herbert","year":2021}
```

## db.count — `GET /api/books/count?author=X`

Output is `{"count": <int64>}` (an object, not a bare scalar) per
`docs/03-nodes/db.count.md`; the workflow projects `nodes.tally.count` into
the response.

```bash
curl localhost:3000/api/books/count?author=Herbert
# → 200 {"count":1}
```

## db.query — `GET /api/book-stats`

Raw SQL `SELECT ... GROUP BY` aggregate. **Route note:** the node-per-route
naming convention would put this at `/api/books/stats`, but Fiber v3 has no
static-vs-`:id` priority — routes are tried in registration order (sorted by
route ID / file name), so `/api/books/stats` would fall through to the
`GET /api/books/:id` (`find-one`) handler first (its file sorts before this
one alphabetically) and 500 trying to treat `"stats"` as a UUID. Rather than
fight the router with filename tricks, this route lives at the
non-nested path `/api/book-stats` instead. (`/api/books/count` and
`/api/books/upsert` happened to sort correctly already and needed no such
rename — verified by running the suite.)

```bash
curl localhost:3000/api/book-stats
# → 200 {"rows":[{"author":"Herbert","n":1},{"author":"Simmons","n":1}]}
```

`n` (from `COUNT(*) AS n`) comes back as a JSON number via the Postgres
driver, not a string — verified by running the suite (see `verify.json`,
which asserts `rows.0.n == 1` as a plain number).

## db.exec — `POST /api/books/retitle`

Raw parameterized `UPDATE`. Output is `{"rows_affected": <count>}` per
`docs/03-nodes/db.exec.md`.

```bash
curl -X POST localhost:3000/api/books/retitle -H 'Content-Type: application/json' \
  -d '{"from": "Hyperion", "to": "Fall of Hyperion"}'
# → 200 {"rows_affected":1}
```
