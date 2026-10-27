package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type Embedding struct {
	VersionID int64
	Model     string
	InputHash string
	Vector    []float32
	Tokens    int64
	CostUSD   float64
	CreatedAt time.Time
}

type EmbedTarget struct {
	VersionID int64
	InputHash string
	Listing   listing.Listing
}

type Neighbor struct {
	VersionID int64
	Source    listing.Source
	SourceID  string
	Distance  float64
}

type PendingFilter struct {
	Model      string
	Collection string
	Limit      int
}

func (s *Store) PendingEmbeddings(ctx context.Context, f PendingFilter) ([]EmbedTarget, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = -1
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.id, v.input_hash, v.data, v.collages
		FROM listings l JOIN versions v ON v.id = l.current_version
		WHERE v.collages IS NOT NULL AND v.input_hash IS NOT NULL
			AND NOT EXISTS (
				SELECT 1 FROM embeddings e
				WHERE e.version_id = v.id AND e.model = ?1 AND e.input_hash = v.input_hash)
			AND (?2 = '' OR EXISTS (
				SELECT 1 FROM collection_observations c
				WHERE c.collection = ?2 AND c.source = l.source AND c.source_id = l.source_id))
		ORDER BY l.observed_at DESC, l.source, l.source_id
		LIMIT ?3`, f.Model, f.Collection, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmbedTarget
	for rows.Next() {
		var (
			t              EmbedTarget
			data, collages string
		)
		if err := rows.Scan(&t.VersionID, &t.InputHash, &data, &collages); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &t.Listing); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(collages), &t.Listing.Collages); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SaveEmbedding(ctx context.Context, e Embedding) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var old int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM embeddings WHERE version_id = ? AND model = ?`, e.VersionID, e.Model).Scan(&old)
		switch {
		case err == nil:
			if _, err := tx.ExecContext(ctx, `DELETE FROM embedding_vectors WHERE rowid = ?`, old); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM embeddings WHERE id = ?`, old); err != nil {
				return err
			}
		case err != sql.ErrNoRows:
			return err
		}
		var id int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO embeddings (version_id, model, input_hash, dimensions, tokens, cost_usd, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			RETURNING id`,
			e.VersionID, e.Model, e.InputHash, len(e.Vector), e.Tokens, e.CostUSD, timestamp(e.CreatedAt)).Scan(&id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO embedding_vectors (rowid, vector, model) VALUES (?, ?, ?)`, id, vectorBlob(e.Vector), e.Model); err != nil {
			return fmt.Errorf("save vector for version %d: %w", e.VersionID, err)
		}
		return nil
	})
}

func (s *Store) Nearest(ctx context.Context, model string, v []float32, k int) ([]Neighbor, error) {
	blob := vectorBlob(v)
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.version_id, ver.source, ver.source_id, vec1_cos_distance(?1, ev.vector) AS distance
		FROM embedding_vectors(?1, ?2) ev
		JOIN embeddings e ON e.id = ev.rowid
		JOIN versions ver ON ver.id = e.version_id
		WHERE ev.model = ?3
		ORDER BY distance
		LIMIT ?4`, blob, fmt.Sprintf("{K:%d}", k), model, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Neighbor
	for rows.Next() {
		var n Neighbor
		if err := rows.Scan(&n.VersionID, &n.Source, &n.SourceID, &n.Distance); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func vectorBlob(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}
