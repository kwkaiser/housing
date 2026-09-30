package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const DefaultBaseURL = "https://api.apify.com/v2"

type Client struct {
	Token             string
	BaseURL           string
	HTTP              *http.Client
	MaxTotalChargeUSD float64
}

func NewClient(token string) *Client {
	return &Client{Token: token, BaseURL: DefaultBaseURL, HTTP: http.DefaultClient}
}

type run struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	StatusMessage    string `json:"statusMessage"`
	DefaultDatasetID string `json:"defaultDatasetId"`
}

func (r run) finished() bool {
	switch r.Status {
	case "SUCCEEDED", "FAILED", "TIMED-OUT", "ABORTED":
		return true
	}
	return false
}

type RunInput struct {
	Input    any
	MaxItems int
}

func (c *Client) Run(ctx context.Context, actorID string, input any) ([]json.RawMessage, error) {
	q := url.Values{"waitForFinish": {"60"}}
	if ri, ok := input.(RunInput); ok {
		input = ri.Input
		if ri.MaxItems > 0 {
			q.Set("maxItems", strconv.Itoa(ri.MaxItems))
		}
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode input: %w", err)
	}

	if c.MaxTotalChargeUSD > 0 {
		q.Set("maxTotalChargeUsd", strconv.FormatFloat(c.MaxTotalChargeUSD, 'f', -1, 64))
	}

	var envelope struct {
		Data run `json:"data"`
	}
	path := "/acts/" + strings.ReplaceAll(actorID, "/", "~") + "/runs?" + q.Encode()
	if err := c.do(ctx, http.MethodPost, path, body, &envelope); err != nil {
		return nil, fmt.Errorf("start %s: %w", actorID, err)
	}

	for !envelope.Data.finished() {
		if err := c.do(ctx, http.MethodGet, "/actor-runs/"+envelope.Data.ID+"?waitForFinish=60", nil, &envelope); err != nil {
			return nil, fmt.Errorf("poll run %s: %w", envelope.Data.ID, err)
		}
	}

	r := envelope.Data
	if r.Status != "SUCCEEDED" {
		return nil, fmt.Errorf("run %s of %s %s: %s", r.ID, actorID, r.Status, r.StatusMessage)
	}

	var items []json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/datasets/"+r.DefaultDatasetID+"/items?format=json&clean=true", nil, &items); err != nil {
		return nil, fmt.Errorf("fetch dataset %s: %w", r.DefaultDatasetID, err)
	}
	return items, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(msg))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
