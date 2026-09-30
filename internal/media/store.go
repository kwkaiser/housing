package media

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type DiskStore struct {
	Root string
}

var _ ImageStore = DiskStore{}

func (s DiskStore) Path(key string) string {
	return filepath.Join(s.Root, filepath.FromSlash(key))
}

func (s DiskStore) Has(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(s.Path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s DiskStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(s.Path(key))
}

func (s DiskStore) Put(_ context.Context, key string, r io.Reader) error {
	path := s.Path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
