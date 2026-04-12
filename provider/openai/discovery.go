package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// modelsResponse is the shape of GET /v1/models from any OpenAI-compatible server.
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// DiscoverModels queries the /v1/models endpoint of an OpenAI-compatible server
// and returns the model IDs it reports. At most 5 seconds is spent on the
// network request; callers that need a custom deadline should use a context with
// a deadline already set.
func DiscoverModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	// Normalise base URL — strip trailing slash, then append /models.
	base := strings.TrimRight(baseURL, "/")
	// If base already ends with /v1 use it as-is; if the caller passed just a
	// host:port, leave it alone — servers differ on exact path.
	endpoint := base + "/models"

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("discovery: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery: GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("discovery: %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("discovery: decode response from %s: %w", endpoint, err)
	}

	ids := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// SelectModel picks a model ID from a list of candidates. If pattern is
// non-empty it is compiled as a case-insensitive regex and the first matching
// ID is returned. If no pattern matches (or pattern is empty), the first
// candidate is returned. Returns "" if candidates is empty.
func SelectModel(candidates []string, pattern string) (string, error) {
	if len(candidates) == 0 {
		return "", nil
	}
	if pattern == "" {
		return candidates[0], nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return "", fmt.Errorf("discovery: invalid model_pattern %q: %w", pattern, err)
	}
	for _, id := range candidates {
		if re.MatchString(id) {
			return id, nil
		}
	}
	// Pattern didn't match anything — fall back to first.
	return candidates[0], nil
}
