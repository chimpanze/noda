# examples/init-example

A [Noda](https://github.com/chimpanze/noda) project.

## Getting Started

This is a committed example, not a freshly scaffolded project, so it ships
`.env.example` rather than a generated `.env` — copy it before running
(`noda init`/`noda_scaffold_project` generate a ready `.env` with a unique
`JWT_SECRET` for you automatically).

```bash
cp .env.example .env

# Start infrastructure
docker compose up -d

# Run in development mode
noda dev

# Run tests
noda test

# Validate config
noda validate --verbose
```

Edit `.env` to point at your own services.

## First request

The scaffold registers `GET /api/hello/:name`:

```bash
curl http://localhost:3000/api/hello/world
# → {"greeting":"Hello, world!"}
```

## Project Structure

```
noda.json           — main configuration (server, services, security)
routes/             — HTTP route definitions
workflows/          — workflow definitions
schemas/            — JSON schemas for validation
tests/              — workflow test suites
migrations/         — database migrations
docker-compose.yml  — local infrastructure
```
