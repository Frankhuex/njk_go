package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HttpClient struct {
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *HttpClient {
	return &HttpClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *HttpClient) DownloadBytes(ctx context.Context, url string) ([]byte, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("http client not available")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
