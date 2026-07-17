# Cookbook: transform nodes

Runnable examples for `transform.set`, `transform.map`, `transform.filter`, `transform.merge`, `transform.delete`, and `transform.validate`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

```bash
noda start --config examples/node-cookbook/transform
```

## transform.set — `POST /api/set`

Computes new fields from expressions and merges them with the input data.

```bash
curl -X POST localhost:3000/api/set -H 'Content-Type: application/json' -d '{"name": "Ada"}'
# → 200 {"greeting":"Hello, Ada!","fixed":42}
```

## transform.map — `POST /api/map`

Applies an expression to each element of a collection; results collect as an array.

```bash
curl -X POST localhost:3000/api/map -H 'Content-Type: application/json' -d '{"items": [1, 2, 3]}'
# → 200 {"items":[2,4,6]}
```

## transform.filter — `POST /api/filter`

Keeps elements where the expression is truthy; discards the rest.

```bash
curl -X POST localhost:3000/api/filter -H 'Content-Type: application/json' -d '{"items": [{"id": 1, "active": true}, {"id": 2, "active": false}, {"id": 3, "active": true}]}'
# → 200 {"items":[{"id":1,"active":true},{"id":3,"active":true}],"count":2}
```

## transform.merge — `POST /api/merge`

Combines two or more arrays or objects into a single collection.

```bash
curl -X POST localhost:3000/api/merge -H 'Content-Type: application/json' -d '{"a": [1, 2], "b": [3]}'
# → 200 {"items":[1,2,3]}
```

## transform.delete — `POST /api/delete`

Removes specified fields from data.

```bash
curl -X POST localhost:3000/api/delete -H 'Content-Type: application/json' -d '{"user": {"name": "Ada", "password": "hunter2"}}'
# → 200 {"name":"Ada"}
```

## transform.validate — `POST /api/validate`

Validates data against a JSON Schema. Routes to the `success` edge if valid, `error` if invalid.

```bash
curl -X POST localhost:3000/api/validate -H 'Content-Type: application/json' -d '{"email": "ada@example.com", "name": "Ada"}'
# → 200 {"valid":true}
curl -X POST localhost:3000/api/validate -H 'Content-Type: application/json' -d '{"name": "Ada"}'
# → 422 {"valid":false}
```
