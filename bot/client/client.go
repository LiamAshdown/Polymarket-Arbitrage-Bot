package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	httpClient *http.Client
	baseUrl    string
	auth       AuthProvider
}

var ErrAPIFailure = errors.New("event not found")

func NewClient(baseUrl string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseUrl:    baseUrl,
	}
}

// SetAuth configures an optional AuthProvider. If set, it will be applied
// to outbound requests for endpoints that require authentication.
func (c *Client) SetAuth(auth AuthProvider) {
	c.auth = auth
}

func (c *Client) doRequest(req *http.Request, result interface{}) error {
	if c.auth != nil {
		if err := c.auth.Apply(req); err != nil {
			return err
		}
	}
	resp, err := c.httpClient.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode > 200 || resp.StatusCode < 200 {
		return fmt.Errorf("API error %d: %s: %w", resp.StatusCode, string(body), ErrAPIFailure)
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) get(ctx context.Context, endpoint string, params url.Values, result interface{}) error {
	reqUrl := c.baseUrl + endpoint
	if len(params) > 0 {
		reqUrl += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return err
	}

	return c.doRequest(req, result)
}

func (c *Client) delete(ctx context.Context, endpoint string, result interface{}) error {
	reqUrl := c.baseUrl + endpoint

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqUrl, nil)
	if err != nil {
		return err
	}

	return c.doRequest(req, result)
}

func (c *Client) getWithHeaders(ctx context.Context, endpoint string, headers map[string]string, result interface{}) error {
	reqUrl := c.baseUrl + endpoint

	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.doRequest(req, result)
}
