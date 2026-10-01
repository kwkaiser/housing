package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var ErrLocked = errors.New("another housing run is in progress")

func (s *Service) Lock() (func(), error) {
	if err := os.MkdirAll(s.cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(s.cfg.DataDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w on %s", ErrLocked, s.cfg.DataDir)
		}
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

func (s *Service) HoldLock() (func(), error) {
	unlock, err := s.Lock()
	if err != nil {
		return nil, err
	}
	s.held.Store(true)
	return func() {
		s.held.Store(false)
		unlock()
	}, nil
}

func (s *Service) runLock() (func(), error) {
	if s.held.Load() {
		return func() {}, nil
	}
	return s.Lock()
}
