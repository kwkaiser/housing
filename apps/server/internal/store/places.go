package store

import (
	"context"
	"errors"
	"time"
)

var ErrPlaceNotFound = errors.New("place not found")

type Place struct {
	ID        int64
	Name      string
	Address   string
	Lat       float64
	Lng       float64
	CreatedAt time.Time
}

func (s *Store) AddPlace(ctx context.Context, p Place) (Place, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO places (name, address, lat, lng, created_at) VALUES (?, ?, ?, ?, ?)`,
		p.Name, p.Address, p.Lat, p.Lng, timestamp(p.CreatedAt))
	if err != nil {
		return Place{}, err
	}
	p.ID, err = res.LastInsertId()
	return p, err
}

func (s *Store) RemovePlace(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM places WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrPlaceNotFound
	}
	return nil
}

func (s *Store) Places(ctx context.Context) ([]Place, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, address, lat, lng, created_at FROM places ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Place{}
	for rows.Next() {
		var p Place
		var created string
		if err := rows.Scan(&p.ID, &p.Name, &p.Address, &p.Lat, &p.Lng, &created); err != nil {
			return nil, err
		}
		if p.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
