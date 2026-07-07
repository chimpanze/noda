CREATE TABLE room_links (
  id         TEXT PRIMARY KEY,
  room_name  TEXT NOT NULL,
  room_type  TEXT NOT NULL,
  token      TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_room_links_room ON room_links(room_name);
