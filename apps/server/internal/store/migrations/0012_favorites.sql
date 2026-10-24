CREATE TABLE favorites (
  source     TEXT NOT NULL,
  source_id  TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (source, source_id),
  FOREIGN KEY (source, source_id) REFERENCES listings (source, source_id)
);
