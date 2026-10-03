CREATE TABLE api_keys (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  hint         TEXT NOT NULL,
  hash         TEXT NOT NULL UNIQUE,
  created_at   TEXT NOT NULL,
  last_used_at TEXT
);
