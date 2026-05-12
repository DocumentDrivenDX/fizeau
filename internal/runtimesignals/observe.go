package runtimesignals

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
)

var (
	defaultCacheOnce sync.Once
	defaultCache     *discoverycache.Cache
	defaultCacheErr  error
)

// Observe refreshes the persistent runtime signal from the in-memory store
// and writes the refreshed snapshot to the default on-disk runtime cache.
func Observe(ctx context.Context, providerName, providerType, baseURL string, headers http.Header, latency time.Duration, callErr error) error {
	providerName = strings.TrimSpace(providerName)
	providerType = strings.TrimSpace(providerType)
	if providerName == "" {
		return nil
	}
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}

	cache, err := defaultRuntimeCache()
	if err != nil {
		return err
	}
	collectType := providerType
	collectBaseURL := baseURL
	if isLocalProviderType(providerType) {
		// Runtime observation should not add a second live HTTP probe for
		// local providers. We still want latency and header-derived data, so
		// collect from the in-memory store using the non-local path.
		collectType = "openai"
		collectBaseURL = ""
	}
	sig, err := Collect(ctx, providerName, CollectInput{Type: collectType, BaseURL: collectBaseURL})
	if err != nil {
		return err
	}
	if callErr != nil {
		now := time.Now().UTC()
		if sig.Status == StatusUnknown {
			sig.Status = StatusDegraded
		}
		sig.LastErrorAt = &now
		sig.LastErrorMsg = callErr.Error()
	}
	return Write(cache, sig)
}

func isLocalProviderType(providerType string) bool {
	switch strings.TrimSpace(providerType) {
	case "ds4", "llama-server", "omlx", "lmstudio", "lucebox", "vllm", "rapid-mlx", "ollama":
		return true
	default:
		return false
	}
}

func defaultRuntimeCache() (*discoverycache.Cache, error) {
	defaultCacheOnce.Do(func() {
		root := strings.TrimSpace(os.Getenv("FIZEAU_CACHE_DIR"))
		if root == "" {
			var err error
			root, err = os.UserCacheDir()
			if err != nil {
				defaultCacheErr = err
				return
			}
			root = filepath.Join(root, "fizeau")
		}
		defaultCache = &discoverycache.Cache{Root: root}
	})
	return defaultCache, defaultCacheErr
}
