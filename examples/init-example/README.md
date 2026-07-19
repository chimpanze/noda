# examples/init-example

A [Noda](https://github.com/chimpanze/noda) project.

## Getting Started

```bash
# Start infrastructure (a ready .env with generated JWT_SECRET was created)
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
