CREATE TABLE runs (
  id          INTEGER PRIMARY KEY,
  kind        TEXT NOT NULL,
  day         TEXT NOT NULL,
  started_at  TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  model       TEXT,
  profiles    TEXT NOT NULL,
  calls       INTEGER NOT NULL,
  cost_usd    REAL NOT NULL,
  failed      INTEGER NOT NULL,
  over_budget INTEGER NOT NULL,
  error       TEXT
);

CREATE INDEX runs_by_day ON runs (day);
