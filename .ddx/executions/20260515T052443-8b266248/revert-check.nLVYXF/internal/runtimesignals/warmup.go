package runtimesignals

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/easel/fizeau/internal/discoverycache"
)

// Warmup synchronously refreshes and writes the runtime signal for providerName
// through the shared discovery cache coordinator. Callers can invoke it from a
// heartbeat without bypassing the same lock/singleflight path used by writes.
func Warmup(ctx context.Context, cache *discoverycache.Cache, providerName string, cfg CollectInput) (Signal, error) {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return Signal{}, nil
	}
	if cache == nil {
		return Signal{}, fmt.Errorf("runtime warmup cache is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	src := CacheSource(providerName)
	var sig Signal
	err := cache.Refresh(src, func(refreshCtx context.Context) ([]byte, error) {
		collected, collectErr := DefaultStore.Collect(refreshCtx, providerName, cfg)
		if collectErr != nil {
			return nil, collectErr
		}
		sig = collected
		data, marshalErr := json.Marshal(collected)
		if marshalErr != nil {
			return nil, marshalErr
		}
		return data, nil
	})
	return sig, err
}
