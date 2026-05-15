package ds4

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/easel/fizeau/internal/provider/utilization"
)

// UtilizationProbe is a thin liveness probe for ds4-server. ds4 deliberately
// has no /health, /metrics, or /slots endpoints — see issue #53 / README.
// The only signal we can pull from the server is GET /v1/models returning
// 200 with the hardcoded "deepseek-v4-flash" entry. Queue depth and active-
// request counts must be tracked client-side; the server serializes requests
// to a single graph worker (max_concurrency=1) regardless of what clients do,
// so synthesizing MaxConcurrency=1 is accurate.
type UtilizationProbe struct {
	baseURL string
	client  *http.Client
	cache   utilization.Cache
}

// NewUtilizationProbe creates a probe for a ds4-server base URL (must end
// with /v1 by convention; the probe joins the rest of the path itself).
func NewUtilizationProbe(baseURL string, client *http.Client) *UtilizationProbe {
	if client == nil {
		client = http.DefaultClient
	}
	return &UtilizationProbe{
		baseURL: baseURL,
		client:  client,
	}
}

// Probe issues GET /v1/models. On 200 with the expected payload shape we
// emit MaxConcurrency=1 (ds4-server's hard limit) with everything else as
// nil. On any failure we fall back to the prior cached sample, or
// utilization.Unknown if we have none.
func (p *UtilizationProbe) Probe(ctx context.Context) utilization.EndpointUtilization {
	if sample, ok := p.probeModels(ctx); ok {
		return p.cache.Remember(sample)
	}
	if stale, ok := p.cache.Stale(); ok {
		return stale
	}
	return utilization.Unknown(utilization.SourceDS4Models)
}

func (p *UtilizationProbe) probeModels(ctx context.Context) (utilization.EndpointUtilization, bool) {
	body, err := p.get(ctx, utilization.ServerRoot(p.baseURL)+"/v1/models")
	if err != nil {
		return utilization.EndpointUtilization{}, false
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &payload); err != nil {
		return utilization.EndpointUtilization{}, false
	}
	if len(payload.Data) == 0 {
		return utilization.EndpointUtilization{}, false
	}
	// ds4 serializes everything to a single graph worker. MaxConcurrency=1
	// is the right backpressure signal for the router; ActiveRequests stays
	// nil because the server does not expose it.
	maxConc := 1
	return utilization.EndpointUtilization{
		MaxConcurrency: &maxConc,
		Source:         utilization.SourceDS4Models,
	}, true
}

func (p *UtilizationProbe) get(ctx context.Context, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
