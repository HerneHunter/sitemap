package sitemap

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

var defaultTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = 10
	t.ResponseHeaderTimeout = 10 * time.Second
	return t
}()

var defaultHTTPClient = &http.Client{
	Transport: defaultTransport,
}

func fetchUrl(ctx context.Context, client *http.Client, url string) (io.Reader, error) {
	if client == nil {
		client = defaultHTTPClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, &HTTPError{URL: url, StatusCode: resp.StatusCode}
	}

	return resp.Body, nil
}
