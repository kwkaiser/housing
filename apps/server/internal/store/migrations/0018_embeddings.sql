CREATE TABLE embeddings (
  id          INTEGER PRIMARY KEY,
  version_id  INTEGER NOT NULL REFERENCES versions (id),
  model       TEXT NOT NULL,
  input_hash  TEXT NOT NULL,
  dimensions  INTEGER NOT NULL,
  tokens      INTEGER NOT NULL,
  cost_usd    REAL NOT NULL,
  created_at  TEXT NOT NULL,
  UNIQUE (version_id, model)
);

CREATE VIRTUAL TABLE embedding_vectors USING vec1(vector, model);

INSERT INTO embedding_vectors(cmd, arg) VALUES ('rebuild', '{index:"flat", distance:"cos"}');
