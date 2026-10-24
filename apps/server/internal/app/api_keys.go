package app

import (
	"context"
	"time"
)

type APIKey struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Hint       string     `json:"hint" doc:"The token's first characters"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type APICreatedKey struct {
	Key   APIKey `json:"key"`
	Token string `json:"token" doc:"The bearer token; it is shown only once"`
}

type APICreateKey struct {
	Name string `json:"name" doc:"What the key is for, such as a device"`
}

func (a *App) apiKeys(ctx context.Context, _ *struct{}) (*struct{ Body []APIKey }, error) {
	ks, err := a.svc.APIKeys(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIKey, len(ks))
	for i, k := range ks {
		out[i] = APIKey{ID: k.ID, Name: k.Name, Hint: k.Hint, CreatedAt: k.CreatedAt}
		if !k.LastUsedAt.IsZero() {
			out[i].LastUsedAt = &k.LastUsedAt
		}
	}
	return &struct{ Body []APIKey }{out}, nil
}

type createdKeyOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         APICreatedKey
}

func (a *App) apiCreateKey(ctx context.Context, in *struct{ Body APICreateKey }) (*createdKeyOutput, error) {
	k, token, err := a.svc.CreateAPIKey(ctx, in.Body.Name)
	if err != nil {
		return nil, a.apiSaveError(ctx, err)
	}
	return &createdKeyOutput{"no-store", APICreatedKey{Key: APIKey{ID: k.ID, Name: k.Name, Hint: k.Hint, CreatedAt: k.CreatedAt}, Token: token}}, nil
}

func (a *App) apiDeleteKey(ctx context.Context, in *struct {
	ID int64 `path:"id"`
}) (*struct{}, error) {
	if err := a.svc.DeleteAPIKey(ctx, in.ID); err != nil {
		return nil, a.apiError(ctx, err)
	}
	return nil, nil
}
