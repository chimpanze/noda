-- auth_users had no DB-level uniqueness on the username stored in
-- metadata->>'username' (only email is UNIQUE, migration 0002); the
-- RealWorld conformance suite registers a duplicate username with a
-- different email and expects 409, which requires this to be enforced.
CREATE UNIQUE INDEX IF NOT EXISTS auth_users_username_uidx ON auth_users ((metadata->>'username'));
