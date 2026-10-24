ALTER TABLE collections ADD COLUMN notify_url TEXT;
ALTER TABLE collections ADD COLUMN notify_min_score REAL NOT NULL DEFAULT 0;

CREATE TABLE notifications (
  collection_id TEXT NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
  source        TEXT NOT NULL,
  source_id     TEXT NOT NULL,
  match         REAL NOT NULL,
  sent_at       TEXT NOT NULL,
  PRIMARY KEY (collection_id, source, source_id)
);
