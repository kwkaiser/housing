CREATE TABLE session_handoffs (
  hash       TEXT PRIMARY KEY,
  challenge  TEXT NOT NULL,
  subject    TEXT NOT NULL,
  name       TEXT NOT NULL,
  email      TEXT NOT NULL,
  expires_at TEXT NOT NULL
);
