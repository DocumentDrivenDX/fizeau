package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ScoredModel is a discovered model with a selection preference score.
// Higher scores are preferred by the auto-selection logic.
type ScoredModel struct {
	// ID is the model identifier returned by the server's /v1/models endpoint.
	ID string
	// CatalogRef is the catalog target ID if this model is recognized in the
	// model catalog for the provider's surface. Empty for unrecognized models.
	CatalogRef string
	// PatternMatch is true when this model matched the configured model_pattern.
	PatternMatch bool
	// Score summarises the selection preference: 3 = catalog-recognized,
	// 2 = pattern-matched, 1 = uncategorized.
	Score int
}

// modelsResponse is the shape of GET /v1/models from any OpenAI-compatible server.
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// DiscoverModels queries the /v1/models endpoint of an OpenAI-compatible server
// and returns the model IDs it reports. At most 5 seconds is spent on the
// network request; callers that need a custom deadline should pass a context
// with a deadline already set.
func DiscoverModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	base := strings.TrimRight(baseURL, "/")
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

// RankModels scores and sorts a list of discovered model IDs by selection
// preference:
//
//   - Score 3 — catalog-recognized: the model ID appears in knownModels (a map
//     from concrete model ID to catalog target ID, e.g. from
//     Catalog.AllConcreteModels). These are explicitly tracked models; prefer
//     them when auto-selecting.
//   - Score 2 — pattern-matched: the model ID matches the case-insensitive
//     pattern regex (pattern == "" means this tier is skipped).
//   - Score 1 — uncategorized: known to the server but not in the catalog or
//     pattern.
//
// Within each score tier, the original server-returned order is preserved.
// Returns an error only if pattern is non-empty and fails to compile.
func RankModels(candidates []string, knownModels map[string]string, pattern string) ([]ScoredModel, error) {
	var patternRe *regexp.Regexp
	if pattern != "" {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("discovery: invalid model_pattern %q: %w", pattern, err)
		}
		patternRe = re
	}

	scored := make([]ScoredModel, 0, len(candidates))
	for _, id := range candidates {
		sm := ScoredModel{ID: id, Score: 1}
		if ref, ok := knownModels[id]; ok {
			sm.CatalogRef = ref
			sm.Score = 3
		} else if patternRe != nil && patternRe.MatchString(id) {
			sm.PatternMatch = true
			sm.Score = 2
		}
		scored = append(scored, sm)
	}

	// Stable sort: higher score first, original order preserved within tier.
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored, nil
}

// SelectModel picks the preferred model ID from a ranked list. Returns ""
// if the list is empty.
func SelectModel(ranked []ScoredModel) string {
	if len(ranked) == 0 {
		return ""
	}
	return ranked[0].ID
}

// NormalizeModelID resolves a caller-supplied model name against the server's
// canonical model catalog (the IDs returned by GET /v1/models). If the name
// matches a catalog entry exactly (case-insensitive), that entry is returned.
// If the name matches exactly one catalog entry by suffix (the part after the
// last '/'), that entry's full ID is returned — this handles the common case
// where a user supplies a bare name like "qwen3-coder-next" but the server
// lists it as "qwen/qwen3-coder-next". Multiple suffix matches produce an
// ambiguity error listing the candidates. Zero matches return the original
// name unchanged.
func NormalizeModelID(requested string, catalog []string) (string, error) {
	reqLower := strings.ToLower(strings.TrimSpace(requested))
	if reqLower == "" {
		return requested, nil
	}

	// Exact match (case-insensitive).
	for _, id := range catalog {
		if strings.EqualFold(id, requested) {
			return id, nil
		}
	}

	// Suffix match: compare requested against the basename (after last '/')
	// of each catalog entry.
	var matches []string
	for _, id := range catalog {
		idLower := strings.ToLower(id)
		slash := strings.LastIndex(idLower, "/")
		if slash < 0 {
			continue // no prefix to strip — already checked via exact match
		}
		basename := idLower[slash+1:]
		if basename == reqLower {
			matches = append(matches, id)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return requested, nil
	default:
		return "", fmt.Errorf("ambiguous model %q: matches %v", requested, matches)
	}
}
