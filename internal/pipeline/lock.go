package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var ErrLocked = errors.New("another housing run is in progress")

func (e *Env) Lock() (func(), error) {
	if err := os.MkdirAll(e.DataDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(e.DataDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w on %s", ErrLocked, e.DataDir)
		}
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
