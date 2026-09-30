package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPFetcher struct {
	HTTP      *http.Client
	UserAgent string
	Attempts  int
	Backoff   time.Duration
}

func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: "Mozilla/5.0",
		Attempts:  3,
		Backoff:   500 * time.Millisecond,
	}
}

var errRetryable = errors.New("retryable")

func (f *HTTPFetcher) Fetch(ctx context.Context, url string) (io.ReadCloser, error) {
	var err error
	for attempt := range max(f.Attempts, 1) {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(f.Backoff << (attempt - 1)):
			}
		}
		var body io.ReadCloser
		body, err = f.fetchOnce(ctx, url)
		if err == nil {
			return body, nil
		}
		if !errors.Is(err, errRetryable) {
			return nil, err
		}
	}
	return nil, err
}

func (f *HTTPFetcher) fetchOnce(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)

	resp, err := f.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("fetch %s: %w: %w", url, errRetryable, err)
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		resp.Body.Close()
		return nil, fmt.Errorf("fetch %s: %w: %s", url, errRetryable, resp.Status)
	case resp.StatusCode != http.StatusOK:
		resp.Body.Close()
		return nil, fmt.Errorf("fetch %s: %s", url, resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch %s: unexpected content type %q", url, ct)
	}
	return resp.Body, nil
}
