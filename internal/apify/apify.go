package apify

import (
	"context"
	"encoding/json"
)

type Runner interface {
	Run(ctx context.Context, actorID string, input any) ([]json.RawMessage, error)
}
