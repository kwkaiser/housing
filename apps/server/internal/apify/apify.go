package apify

import (
	"context"
	"encoding/json"
)

type Runner interface {
	Run(ctx context.Context, actorID string, input any) ([]json.RawMessage, error)
}

const DefaultConcurrency = 4

type limitedRunner struct {
	runner Runner
	sem    chan struct{}
}

func Limit(r Runner, n int) Runner {
	return limitedRunner{runner: r, sem: make(chan struct{}, max(n, 1))}
}

func (l limitedRunner) Run(ctx context.Context, actorID string, input any) ([]json.RawMessage, error) {
	select {
	case l.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-l.sem }()
	return l.runner.Run(ctx, actorID, input)
}
