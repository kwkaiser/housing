package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const DefaultTimeout = 10 * time.Second

type Message struct {
	Title    string
	Body     string
	Click    string
	Attach   string
	Tags     []string
	Priority int
}

type Notifier interface {
	Notify(ctx context.Context, m Message) error
}

type Ntfy struct {
	URL    string
	Client *http.Client
}

func NewNtfy(topicURL string) *Ntfy {
	return &Ntfy{URL: topicURL, Client: &http.Client{Timeout: DefaultTimeout}}
}

type ntfyMessage struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title,omitempty"`
	Message  string   `json:"message"`
	Click    string   `json:"click,omitempty"`
	Attach   string   `json:"attach,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Priority int      `json:"priority,omitempty"`
}

func (n *Ntfy) Notify(ctx context.Context, m Message) error {
	base, topic, err := SplitTopic(n.URL)
	if err != nil {
		return err
	}
	body, err := json.Marshal(ntfyMessage{
		Topic: topic, Title: m.Title, Message: m.Body, Click: m.Click, Attach: m.Attach, Tags: m.Tags, Priority: m.Priority,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := n.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ntfy: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

func SplitTopic(topicURL string) (string, string, error) {
	u, err := url.Parse(topicURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid ntfy url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", "", fmt.Errorf("invalid ntfy url %q: want an http(s) topic url like https://ntfy.sh/my-topic", u.Redacted())
	}
	dir, topic := path.Split(strings.TrimSuffix(u.Path, "/"))
	if topic == "" {
		return "", "", fmt.Errorf("invalid ntfy url %q: missing topic", u.Redacted())
	}
	u.Path, u.RawPath = dir, ""
	return u.String(), topic, nil
}
