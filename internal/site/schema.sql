CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE collections (
  id       TEXT PRIMARY KEY,
  position INTEGER NOT NULL,
  mode     TEXT NOT NULL,
  location TEXT NOT NULL,
  profiles TEXT NOT NULL,
  models   TEXT NOT NULL
);

CREATE TABLE profiles (
  collection TEXT NOT NULL,
  id         TEXT NOT NULL,
  position   INTEGER NOT NULL,
  name       TEXT NOT NULL,
  summary    TEXT NOT NULL,
  wants      TEXT NOT NULL,
  avoids     TEXT NOT NULL,
  refs       TEXT NOT NULL,
  PRIMARY KEY (collection, id)
);

CREATE TABLE days (
  collection TEXT NOT NULL,
  day        TEXT NOT NULL,
  listings   INTEGER NOT NULL,
  graded     INTEGER NOT NULL,
  PRIMARY KEY (collection, day)
);

CREATE TABLE rows (
  collection           TEXT NOT NULL,
  day                  TEXT NOT NULL,
  ord                  INTEGER NOT NULL,
  source               TEXT NOT NULL,
  source_id            TEXT NOT NULL,
  url                  TEXT NOT NULL,
  address              TEXT NOT NULL,
  offer                TEXT NOT NULL,
  price_cents          INTEGER NOT NULL,
  previous_price_cents INTEGER,
  first_seen           TEXT NOT NULL,
  beds                 INTEGER,
  profile              TEXT NOT NULL,
  match                REAL NOT NULL,
  calibrated           INTEGER NOT NULL,
  score                REAL NOT NULL,
  coverage             REAL NOT NULL,
  vibe                 REAL NOT NULL,
  stale                INTEGER NOT NULL,
  missing              TEXT NOT NULL,
  dealbreakers         TEXT NOT NULL,
  avoids               TEXT NOT NULL,
  summary              TEXT NOT NULL,
  grades               TEXT NOT NULL,
  also_listed          TEXT NOT NULL,
  PRIMARY KEY (collection, day, source, source_id)
);

CREATE INDEX rows_by_day ON rows (collection, day, ord);
