CREATE TABLE listings (
  source          TEXT NOT NULL,
  source_id       TEXT NOT NULL,
  url             TEXT NOT NULL,
  offer           TEXT NOT NULL,
  price_cents     INTEGER NOT NULL,
  currency        TEXT NOT NULL,
  observed_at     TEXT NOT NULL,
  current_version INTEGER,
  PRIMARY KEY (source, source_id)
);

CREATE TABLE versions (
  id           INTEGER PRIMARY KEY,
  source       TEXT NOT NULL,
  source_id    TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  input_hash   TEXT,
  address      TEXT NOT NULL,
  city         TEXT,
  state        TEXT,
  postal_code  TEXT,
  lat          REAL,
  lng          REAL,
  beds         INTEGER,
  baths        REAL,
  sqft         INTEGER,
  collages     TEXT,
  data         TEXT NOT NULL,
  raw          TEXT,
  created_at   TEXT NOT NULL,
  UNIQUE (source, source_id, content_hash),
  FOREIGN KEY (source, source_id) REFERENCES listings (source, source_id)
);

CREATE INDEX versions_by_input ON versions (input_hash);

CREATE TABLE observations (
  day         TEXT NOT NULL,
  source      TEXT NOT NULL,
  source_id   TEXT NOT NULL,
  version_id  INTEGER NOT NULL REFERENCES versions (id),
  price_cents INTEGER NOT NULL,
  currency    TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  PRIMARY KEY (day, source, source_id),
  FOREIGN KEY (source, source_id) REFERENCES listings (source, source_id)
);

CREATE INDEX observations_by_listing ON observations (source, source_id, day);

CREATE TABLE assessments (
  id           INTEGER PRIMARY KEY,
  source       TEXT NOT NULL,
  source_id    TEXT NOT NULL,
  profile_id   TEXT NOT NULL,
  model        TEXT NOT NULL,
  profile_hash TEXT NOT NULL,
  input_hash   TEXT NOT NULL,
  score        REAL NOT NULL,
  coverage     REAL NOT NULL,
  vibe         INTEGER NOT NULL,
  assessed_at  TEXT NOT NULL,
  cost_usd     REAL NOT NULL,
  data         TEXT NOT NULL,
  UNIQUE (source, source_id, profile_id, model, profile_hash, input_hash),
  FOREIGN KEY (source, source_id) REFERENCES listings (source, source_id)
);

CREATE INDEX assessments_by_input ON assessments (input_hash, profile_id, model);
