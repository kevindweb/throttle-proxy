package proxymw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	InstantQueryEndpoint = "/api/v1/query"
)

// ValueFromPromQL queries the prometheus instant API for the prometheus query.
// Throws an error if the response is not a single value.
func ValueFromPromQL(
	ctx context.Context, client *http.Client, endpoint, query string,
) (float64, error) {
	u, err := url.Parse(endpoint + InstantQueryEndpoint)
	if err != nil {
		return 0, fmt.Errorf("parse monitor URL: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("execute request: %w", err)
	}

	defer resp.Body.Close() //nolint:errcheck // ignore body close
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var prometheusResp PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&prometheusResp); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	results := prometheusResp.Data.Result
	if len(results) != 1 {
		return 0, fmt.Errorf("backpressure query must return exactly one value: %s", query)
	}

	res := float64(results[0].Value)
	if res < 0 {
		return 0, fmt.Errorf("backpressure query (%s) must have non-negative value: %f", query, res)
	}

	return res, nil
}
