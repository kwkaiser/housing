package pipeline

import (
	"fmt"
	"io"
	"sync"

	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type Env struct {
	DataDir     string
	ProfilesDir string
	Config      config.Config
	Out         io.Writer

	mu sync.Mutex
}

func (e *Env) printf(format string, args ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fmt.Fprintf(e.Out, format, args...)
}

func (e *Env) persister() jsonfile.Persister {
	return jsonfile.Persister{}
}

func (e *Env) images() media.DiskStore {
	return media.DiskStore{Root: e.DataDir}
}

func (e *Env) profiles() profile.Store {
	return profile.Store{Root: e.ProfilesDir}
}
