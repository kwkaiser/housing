package openrouter

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	sdk "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
	"github.com/OpenRouterTeam/go-sdk/retry"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"
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
	Model        string
	Content      string
	CostUSD      float64
	FinishReason string
}

var ErrEmptyResponse = errors.New("openrouter returned no content")

func (r Response) Text() (string, error) {
	if r.Content == "" {
		if r.FinishReason != "" {
			return "", fmt.Errorf("%w (finish reason %q)", ErrEmptyResponse, r.FinishReason)
		}
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
	sdk     *sdk.OpenRouter
	limiter *rate.Limiter
}

var _ Completer = (*Client)(nil)

type settings struct {
	serverURL    string
	retryMax     int
	retryWaitMin time.Duration
	retryWaitMax time.Duration
	perSecond    float64
	burst        int
}

type Option func(*settings)

func WithServerURL(url string) Option {
	return func(s *settings) { s.serverURL = url }
}

func WithRetries(max int, waitMin, waitMax time.Duration) Option {
	return func(s *settings) { s.retryMax, s.retryWaitMin, s.retryWaitMax = max, waitMin, waitMax }
}

func WithRateLimit(perSecond float64, burst int) Option {
	return func(s *settings) { s.perSecond, s.burst = perSecond, burst }
}

func NewClient(apiKey string, opts ...Option) *Client {
	s := settings{
		retryMax:     4,
		retryWaitMin: time.Second,
		retryWaitMax: 30 * time.Second,
		perSecond:    4,
		burst:        4,
	}
	for _, o := range opts {
		o(&s)
	}

	httpClient := retryablehttp.NewClient()
	httpClient.RetryMax = s.retryMax
	httpClient.RetryWaitMin = s.retryWaitMin
	httpClient.RetryWaitMax = s.retryWaitMax
	httpClient.HTTPClient.Timeout = 5 * time.Minute
	httpClient.Logger = nil
	httpClient.ErrorHandler = retryablehttp.PassthroughErrorHandler

	sdkOpts := []sdk.SDKOption{
		sdk.WithSecurity(apiKey),
		sdk.WithXTitle("housing"),
		sdk.WithClient(httpClient.StandardClient()),
		sdk.WithRetryConfig(retry.Config{Strategy: "none"}),
	}
	if s.serverURL != "" {
		sdkOpts = append(sdkOpts, sdk.WithServerURL(s.serverURL))
	}

	limit := rate.Inf
	if s.perSecond > 0 {
		limit = rate.Limit(s.perSecond)
	}
	return &Client{sdk: sdk.New(sdkOpts...), limiter: rate.NewLimiter(limit, max(s.burst, 1))}
}

func (c *Client) Complete(ctx context.Context, req Request) (Response, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return Response{}, err
	}
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
	if fr := r.Choices[0].FinishReason; fr != nil {
		out.FinishReason = string(*fr)
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
