ALTER TABLE runs ADD COLUMN collection TEXT;

CREATE TABLE collection_observations (
  collection TEXT NOT NULL,
  day        TEXT NOT NULL,
  source     TEXT NOT NULL,
  source_id  TEXT NOT NULL,
  PRIMARY KEY (collection, day, source, source_id),
  FOREIGN KEY (day, source, source_id) REFERENCES observations (day, source, source_id)
);
