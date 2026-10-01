ALTER TABLE collections ADD COLUMN schedule TEXT;

CREATE TABLE jobs (
  id            INTEGER PRIMARY KEY,
  kind          TEXT NOT NULL,
  trigger       TEXT NOT NULL,
  collection_id TEXT,
  profile_id    TEXT,
  params        TEXT NOT NULL,
  status        TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  started_at    TEXT,
  finished_at   TEXT,
  error         TEXT NOT NULL DEFAULT '',
  cost_usd      REAL NOT NULL DEFAULT 0,
  result        TEXT
);

CREATE INDEX jobs_by_created ON jobs (created_at);
CREATE INDEX jobs_by_status ON jobs (status, id);
CREATE INDEX jobs_by_collection ON jobs (collection_id, kind, status);
CREATE INDEX jobs_by_collection_created ON jobs (collection_id, kind, created_at);
CREATE INDEX jobs_by_profile ON jobs (profile_id, created_at);

CREATE TABLE job_events (
  job_id  INTEGER NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
  seq     INTEGER NOT NULL,
  at      TEXT NOT NULL,
  stage   TEXT NOT NULL,
  message TEXT NOT NULL,
  PRIMARY KEY (job_id, seq)
);
