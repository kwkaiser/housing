CREATE TABLE places (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  address    TEXT NOT NULL,
  lat        REAL NOT NULL,
  lng        REAL NOT NULL,
  created_at TEXT NOT NULL
);
