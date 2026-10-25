CREATE TABLE sessions (
  hash       TEXT PRIMARY KEY,
  subject    TEXT NOT NULL,
  name       TEXT NOT NULL,
  email      TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE INDEX sessions_expires_at ON sessions (expires_at);
