package openrouter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const DefaultBaseURL = "https://openrouter.ai/api/v1"

type Completer interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
	Usage          *UsageOptions   `json:"usage,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content []Part `json:"content"`
}

type Part struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type UsageOptions struct {
	Include bool `json:"include"`
}

type Response struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Content string `json:"content"`
	} `json:"message"`
}

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
}

var ErrEmptyResponse = errors.New("openrouter returned no content")

func (r Response) Text() (string, error) {
	if len(r.Choices) == 0 || r.Choices[0].Message.Content == "" {
		return "", ErrEmptyResponse
	}
	return r.Choices[0].Message.Content, nil
}

func TextPart(text string) Part {
	return Part{Type: "text", Text: text}
}

func ImagePart(mediaType string, data []byte) Part {
	return Part{Type: "image_url", ImageURL: &ImageURL{URL: "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)}}
}

func JSONSchemaFormat(name string, schema json.RawMessage) *ResponseFormat {
	return &ResponseFormat{Type: "json_schema", JSONSchema: &JSONSchema{Name: name, Strict: true, Schema: schema}}
}

type Client struct {
	APIKey  string
	BaseURL string
	Title   string
	HTTP    *http.Client
}

var _ Completer = (*Client)(nil)

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		Title:   "housing",
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *Client) Complete(ctx context.Context, req Request) (Response, error) {
	if req.Usage == nil {
		req.Usage = &UsageOptions{Include: true}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if c.Title != "" {
		httpReq.Header.Set("X-Title", c.Title)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	var apiErr struct {
		Error *struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != nil {
		return Response{}, fmt.Errorf("openrouter: %s (%v)", apiErr.Error.Message, apiErr.Error.Code)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("openrouter: %s: %s", resp.Status, bytes.TrimSpace(data))
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return Response{}, fmt.Errorf("decode openrouter response: %w", err)
	}
	return out, nil
}
