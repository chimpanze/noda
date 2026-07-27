-- Example migration. Tables are not created automatically — add a migration like
-- this for every table your workflows read or write, then run: noda migrate up
--
-- Filenames follow <YYYYMMDDHHMMSS>_<name>.(up|down).sql;
-- `noda migrate create <name>` stamps the timestamp for you.
CREATE TABLE items (
  id         UUID PRIMARY KEY,
  name       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
