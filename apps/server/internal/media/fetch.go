package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

type HTTPFetcher struct {
	Client    *retryablehttp.Client
	UserAgent string
}

func NewHTTPFetcher() *HTTPFetcher {
	c := retryablehttp.NewClient()
	c.RetryMax = 2
	c.RetryWaitMin = 500 * time.Millisecond
	c.RetryWaitMax = 5 * time.Second
	c.HTTPClient.Timeout = 30 * time.Second
	c.Logger = nil
	return &HTTPFetcher{Client: c, UserAgent: "Mozilla/5.0"}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch %s: %s", url, resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch %s: unexpected content type %q", url, ct)
	}
	return resp.Body, nil
}
