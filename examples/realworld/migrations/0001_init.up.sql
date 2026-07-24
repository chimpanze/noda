-- Domain tables for the RealWorld/Conduit example.
--
-- The `auth` plugin owns and auto-migrates its own users/sessions/tokens
-- tables (see plugins/auth) — this migration must NOT create a users table.
--
-- author_id / user_id / follower_id / followee_id are TEXT to match the
-- auth plugin's user id column. Confirmed via
-- `grep -rn "id" plugins/auth/create_user.go` -> the auth plugin generates
-- ids with `uuid.NewString()` (github.com/google/uuid), i.e. the users.id
-- column is a TEXT/VARCHAR UUID string, not a Postgres UUID or integer
-- type. TEXT here is therefore an exact type match, not just a
-- cast-compatible approximation.
CREATE TABLE IF NOT EXISTS articles (
    id          BIGSERIAL PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    author_id   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS tags (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS article_tags (
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    tag        TEXT NOT NULL,
    PRIMARY KEY (article_id, tag)
);
CREATE TABLE IF NOT EXISTS favorites (
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL,
    PRIMARY KEY (article_id, user_id)
);
CREATE TABLE IF NOT EXISTS follows (
    follower_id TEXT NOT NULL,
    followee_id TEXT NOT NULL,
    PRIMARY KEY (follower_id, followee_id)
);
CREATE TABLE IF NOT EXISTS comments (
    id         BIGSERIAL PRIMARY KEY,
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_articles_author ON articles(author_id);
CREATE INDEX IF NOT EXISTS idx_articles_created ON articles(created_at DESC);
