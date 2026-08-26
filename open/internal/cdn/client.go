package cdn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	endpoint *url.URL
	http     *http.Client
}

type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func New(endpoint string) *Client {
	parsed, _ := url.Parse(endpoint)
	return &Client{
		endpoint: parsed,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Flush(ctx context.Context, gameID int) error {
	requestURL := *c.endpoint
	query := requestURL.Query()
	query.Set("zone_id", strconv.Itoa(gameID))
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create CDN request: %w", err)
	}
	result, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("flush CDN for game%d: %w", gameID, err)
	}
	defer result.Body.Close()

	if result.StatusCode != http.StatusOK {
		return fmt.Errorf("flush CDN for game%d: unexpected HTTP status %s", gameID, result.Status)
	}
	var body response
	if err := json.NewDecoder(result.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode CDN response: %w", err)
	}
	if body.Code != 0 {
		return fmt.Errorf("flush CDN for game%d: %s", gameID, body.Message)
	}
	return nil
}
