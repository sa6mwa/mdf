package mdf

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// HTTPRenderRequest configures HTTPRender.
type HTTPRenderRequest struct {
	// URL is the HTTP or HTTPS Markdown source.
	URL string
	// Client is used to fetch URL. If nil, http.DefaultClient is used.
	Client *http.Client
	// Writer receives ANSI-rendered Markdown.
	Writer io.Writer
	// Width is the target terminal width in cells. Values <= 0 disable wrapping.
	Width int
	// Theme controls semantic ANSI styles. If nil, DefaultTheme is used.
	Theme Theme
	// Options configures renderer behavior such as OSC 8 links and tables.
	Options []RenderOption
}

// HTTPRender fetches Markdown over HTTP(S) and streams ANSI output.
func HTTPRender(ctx context.Context, req HTTPRenderRequest) error {
	if req.URL == "" {
		return fmt.Errorf("stream http: URL is required")
	}
	if req.Writer == nil {
		return fmt.Errorf("stream http: Writer is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return fmt.Errorf("stream http: build request: %w", err)
	}
	if httpReq.URL.Scheme != "http" && httpReq.URL.Scheme != "https" {
		return fmt.Errorf("stream http: unsupported scheme %q", httpReq.URL.Scheme)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("stream http: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("stream http: status %s", resp.Status)
	}
	return Render(RenderRequest{
		Reader:  resp.Body,
		Writer:  req.Writer,
		Width:   req.Width,
		Theme:   req.Theme,
		Options: req.Options,
	})
}
