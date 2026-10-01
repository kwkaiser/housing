CREATE TABLE profiles (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL,
  name       TEXT NOT NULL,
  summary    TEXT NOT NULL,
  notes      TEXT NOT NULL,
  want       TEXT NOT NULL,
  avoid      TEXT NOT NULL,
  ignore     TEXT NOT NULL,
  searches   TEXT NOT NULL,
  drafted    TEXT,
  updated_at TEXT NOT NULL
);

CREATE TABLE profile_references (
  profile_id TEXT NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  source     TEXT NOT NULL,
  source_id  TEXT NOT NULL,
  url        TEXT NOT NULL,
  collages   TEXT NOT NULL,
  version_id INTEGER NOT NULL REFERENCES versions (id),
  PRIMARY KEY (profile_id, source, source_id),
  UNIQUE (profile_id, position),
  FOREIGN KEY (source, source_id) REFERENCES listings (source, source_id)
);

CREATE TABLE collections (
  id               TEXT PRIMARY KEY,
  mode             TEXT NOT NULL,
  sources          TEXT NOT NULL,
  search           TEXT NOT NULL,
  model            TEXT NOT NULL,
  max_run_cost_usd REAL NOT NULL,
  updated_at       TEXT NOT NULL
);

CREATE TABLE collection_profiles (
  collection_id TEXT NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
  position      INTEGER NOT NULL,
  profile_id    TEXT NOT NULL REFERENCES profiles (id),
  PRIMARY KEY (collection_id, position)
);

CREATE INDEX collection_profiles_by_profile ON collection_profiles (profile_id);
