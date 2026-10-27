package openrouter

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/OpenRouterTeam/go-sdk/models/operations"
)

type Embedder interface {
	Embed(ctx context.Context, req EmbedRequest) (Embedding, error)
}

type EmbedRequest struct {
	Model      string
	Parts      []Part
	Dimensions int64
}

type Embedding struct {
	Model   string
	Vector  []float32
	CostUSD float64
	Tokens  int64
}

var (
	_ Embedder = (*Client)(nil)

	ErrEmptyEmbedding = errors.New("openrouter returned no embedding")
)

func (c *Client) Embed(ctx context.Context, req EmbedRequest) (Embedding, error) {
	body := operations.CreateEmbeddingsRequest{
		Model: req.Model,
		Input: operations.CreateInputUnionArrayOfInput([]operations.Input{{Content: embedContent(req.Parts)}}),
	}
	if req.Dimensions > 0 {
		body.Dimensions = &req.Dimensions
	}
	format := operations.EncodingFormatFloat
	body.EncodingFormat = &format

	if err := c.limiter.Wait(ctx); err != nil {
		return Embedding{}, err
	}
	res, err := c.sdk.Embeddings.Generate(ctx, body)
	if err != nil {
		return Embedding{}, fmt.Errorf("openrouter: %w", err)
	}
	if res == nil || res.CreateEmbeddingsResponseBody == nil {
		return Embedding{}, ErrEmptyEmbedding
	}
	return toEmbedding(*res.CreateEmbeddingsResponseBody, req.Dimensions)
}

func embedContent(parts []Part) []operations.Content {
	out := make([]operations.Content, 0, len(parts))
	for _, p := range parts {
		if p.Data != nil {
			out = append(out, operations.CreateContentImageURL(operations.ContentImageURL{
				Type:     operations.TypeImageURLImageURL,
				ImageURL: operations.ImageURL{URL: "data:" + p.MediaType + ";base64," + base64.StdEncoding.EncodeToString(p.Data)},
			}))
			continue
		}
		out = append(out, operations.CreateContentText(operations.ContentText{Type: operations.TypeTextText, Text: p.Text}))
	}
	return out
}

func toEmbedding(r operations.CreateEmbeddingsResponseBody, dimensions int64) (Embedding, error) {
	if len(r.Data) == 0 || len(r.Data[0].Embedding.ArrayOfNumber) == 0 {
		return Embedding{}, ErrEmptyEmbedding
	}
	values := r.Data[0].Embedding.ArrayOfNumber
	if dimensions > 0 && int64(len(values)) != dimensions {
		return Embedding{}, fmt.Errorf("openrouter returned %d dimensions, want %d", len(values), dimensions)
	}
	out := Embedding{Model: r.Model, Vector: make([]float32, len(values))}
	for i, v := range values {
		out.Vector[i] = float32(v)
	}
	if u := r.Usage; u != nil {
		out.Tokens = u.PromptTokens
		if u.Cost != nil {
			out.CostUSD = *u.Cost
		}
	}
	return out, nil
}
