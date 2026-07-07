CREATE TABLE drops (
  id         TEXT PRIMARY KEY,
  text       TEXT,
  file_name  TEXT,
  file_key   TEXT,
  file_size  BIGINT,
  file_mime  TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT drop_has_content CHECK (text IS NOT NULL OR file_key IS NOT NULL)
);
CREATE INDEX idx_drops_created ON drops(created_at DESC);

CREATE TABLE share_links (
  id         TEXT PRIMARY KEY,
  drop_id    TEXT NOT NULL REFERENCES drops(id) ON DELETE CASCADE,
  token      TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_share_links_drop ON share_links(drop_id);
