# Brief 03 — Data: notes CRUD on Postgres

You are building a web API with Noda backed by PostgreSQL.

**What I want:** A "notes" resource with create, read-one, list, and delete
endpoints (`POST /notes`, `GET /notes/:id`, `GET /notes`, `DELETE /notes/:id`).
Notes have an `id`, a `title`, and a `body`, and are stored in Postgres. Include
whatever database migration is needed to create the table.

**Done looks like:** The project validates cleanly, the migration creates the
notes table, and the four endpoints read and write notes in the database.
