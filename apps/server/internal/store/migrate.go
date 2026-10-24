package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func migrations() ([]migration, error) {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, name := range names {
		base := path.Base(name)
		prefix, _, ok := strings.Cut(base, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %s: name must start with a number and an underscore", base)
		}
		b, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: version, name: base, sql: string(b)})
	}
	slices.SortFunc(out, func(a, b migration) int { return a.version - b.version })
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration %s: expected version %d", m.name, i+1)
		}
	}
	return out, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	ms, err := migrations()
	if err != nil {
		return err
	}
	current, err := userVersion(ctx, db)
	if err != nil {
		return err
	}
	if current > len(ms) {
		return fmt.Errorf("database schema is at version %d but this build only knows %d; upgrade housing", current, len(ms))
	}
	for _, m := range ms[current:] {
		if err := apply(ctx, db, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
	}
	return nil
}

func apply(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return err
	}
	return tx.Commit()
}

func userVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v int
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}
