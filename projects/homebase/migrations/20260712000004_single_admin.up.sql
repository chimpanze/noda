-- Homebase is single-owner by design: guests join via tokens, never as
-- auth_users rows. This closes the /setup count-then-create race (#304):
-- the loser's INSERT hits the index, auth.create_user maps the unique
-- violation to its "exists" output, and setup.json already answers 403.
CREATE UNIQUE INDEX IF NOT EXISTS auth_users_single_row ON auth_users ((true));
