package openrouter

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	sdk "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
	"github.com/OpenRouterTeam/go-sdk/retry"
)

type Completer interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	Model       string
	System      string
	User        []Part
	Schema      *Schema
	Temperature *float64
}

type Part struct {
	Text      string
	MediaType string
	Data      []byte
}

type Schema struct {
	Name   string
	Schema map[string]any
}

type Response struct {
	Model   string
	Content string
	CostUSD float64
}

var ErrEmptyResponse = errors.New("openrouter returned no content")

func (r Response) Text() (string, error) {
	if r.Content == "" {
		return "", ErrEmptyResponse
	}
	return r.Content, nil
}

func TextPart(text string) Part {
	return Part{Text: text}
}

func ImagePart(mediaType string, data []byte) Part {
	return Part{MediaType: mediaType, Data: data}
}

type Client struct {
	sdk *sdk.OpenRouter
}

var _ Completer = (*Client)(nil)

type Option func(*[]sdk.SDKOption)

func WithServerURL(url string) Option {
	return func(o *[]sdk.SDKOption) { *o = append(*o, sdk.WithServerURL(url)) }
}

func WithHTTPClient(c *http.Client) Option {
	return func(o *[]sdk.SDKOption) { *o = append(*o, sdk.WithClient(c)) }
}

func WithMaxRetryTime(d time.Duration) Option {
	return func(o *[]sdk.SDKOption) { *o = append(*o, sdk.WithRetryConfig(retryConfig(d))) }
}

func NewClient(apiKey string, opts ...Option) *Client {
	sdkOpts := []sdk.SDKOption{
		sdk.WithSecurity(apiKey),
		sdk.WithXTitle("housing"),
		sdk.WithTimeout(5 * time.Minute),
		sdk.WithRetryConfig(retryConfig(2 * time.Minute)),
	}
	for _, o := range opts {
		o(&sdkOpts)
	}
	return &Client{sdk: sdk.New(sdkOpts...)}
}

func retryConfig(maxElapsed time.Duration) retry.Config {
	return retry.Config{
		Strategy: "backoff",
		Backoff: &retry.BackoffStrategy{
			InitialInterval: 1000,
			MaxInterval:     20000,
			Exponent:        2,
			MaxElapsedTime:  int(maxElapsed.Milliseconds()),
		},
		RetryConnectionErrors: true,
	}
}

func (c *Client) Complete(ctx context.Context, req Request) (Response, error) {
	chat := components.ChatRequest{
		Model:    &req.Model,
		Messages: messages(req),
	}
	if req.Temperature != nil {
		chat.Temperature = optionalnullable.From(req.Temperature)
	}
	if req.Schema != nil {
		strict := true
		format := components.CreateResponseFormatJSONSchema(components.ChatFormatJSONSchemaConfig{
			Type: components.ChatFormatJSONSchemaConfigTypeJSONSchema,
			JSONSchema: components.ChatJSONSchemaConfig{
				Name:   req.Schema.Name,
				Schema: req.Schema.Schema,
				Strict: optionalnullable.From(&strict),
			},
		})
		chat.ResponseFormat = &format
	}

	res, err := c.sdk.Chat.Send(ctx, chat, nil)
	if err != nil {
		return Response{}, fmt.Errorf("openrouter: %w", err)
	}
	if res == nil || res.ChatResult == nil {
		return Response{}, ErrEmptyResponse
	}
	return toResponse(*res.ChatResult), nil
}

func messages(req Request) []components.ChatMessages {
	var out []components.ChatMessages
	if req.System != "" {
		out = append(out, components.CreateChatMessagesSystem(components.ChatSystemMessage{
			Role:    components.ChatSystemMessageRoleSystem,
			Content: components.CreateChatSystemMessageContentStr(req.System),
		}))
	}
	items := make([]components.ChatContentItems, 0, len(req.User))
	for _, p := range req.User {
		if p.Data != nil {
			items = append(items, components.CreateChatContentItemsImageURL(components.ChatContentImage{
				Type: components.ChatContentImageTypeImageURL,
				ImageURL: components.ChatContentImageImageURL{
					URL: "data:" + p.MediaType + ";base64," + base64.StdEncoding.EncodeToString(p.Data),
				},
			}))
			continue
		}
		items = append(items, components.CreateChatContentItemsText(components.ChatContentText{
			Type: components.ChatContentTextTypeText,
			Text: p.Text,
		}))
	}
	return append(out, components.CreateChatMessagesUser(components.ChatUserMessage{
		Role:    components.ChatUserMessageRoleUser,
		Content: components.CreateChatUserMessageContentArrayOfChatContentItems(items),
	}))
}

func toResponse(r components.ChatResult) Response {
	out := Response{Model: r.Model}
	if r.Usage != nil {
		if cost, ok := r.Usage.Cost.GetOrZero(); ok {
			out.CostUSD = cost
		}
	}
	if len(r.Choices) == 0 {
		return out
	}
	content, ok := r.Choices[0].Message.Content.GetOrZero()
	if !ok {
		return out
	}
	if content.Str != nil {
		out.Content = *content.Str
		return out
	}
	for _, item := range content.ArrayOfChatContentItems {
		if item.ChatContentText != nil {
			out.Content += item.ChatContentText.Text
		}
	}
	return out
}
