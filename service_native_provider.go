package agent

import (
	"fmt"
	"strings"

	agentcore "github.com/DocumentDrivenDX/agent/internal/core"
	"github.com/DocumentDrivenDX/agent/internal/provider/anthropic"
	"github.com/DocumentDrivenDX/agent/internal/provider/lmstudio"
	"github.com/DocumentDrivenDX/agent/internal/provider/ollama"
	"github.com/DocumentDrivenDX/agent/internal/provider/omlx"
	oaiProvider "github.com/DocumentDrivenDX/agent/internal/provider/openai"
	"github.com/DocumentDrivenDX/agent/internal/provider/openrouter"
)

type nativeProviderResolution struct {
	Provider agentcore.Provider
	Name     string
	Entry    ServiceProviderEntry
}

func (s *service) resolveConfiguredNativeProvider(req ServiceExecuteRequest) nativeProviderResolution {
	sc := s.opts.ServiceConfig
	if sc == nil {
		return nativeProviderResolution{}
	}
	name, entry, ok := selectConfiguredNativeProvider(sc, req)
	if !ok {
		return nativeProviderResolution{}
	}
	if req.Model != "" {
		entry.Model = req.Model
	}
	provider := buildNativeProvider(name, entry)
	if provider == nil {
		return nativeProviderResolution{Name: name, Entry: entry}
	}
	return nativeProviderResolution{Provider: provider, Name: name, Entry: entry}
}

func selectConfiguredNativeProvider(sc ServiceConfig, req ServiceExecuteRequest) (string, ServiceProviderEntry, bool) {
	if req.Provider != "" {
		if entry, ok := sc.Provider(req.Provider); ok {
			return req.Provider, entry, true
		}
	}

	wantedType := requestedNativeProviderType(req)
	if wantedType != "" {
		if name := sc.DefaultProviderName(); name != "" {
			if entry, ok := sc.Provider(name); ok && normalizeServiceProviderType(entry.Type) == wantedType {
				return name, entry, true
			}
		}
		for _, name := range sc.ProviderNames() {
			entry, ok := sc.Provider(name)
			if ok && normalizeServiceProviderType(entry.Type) == wantedType {
				return name, entry, true
			}
		}
	}

	if req.Provider == "" && wantedType == "" {
		name := sc.DefaultProviderName()
		if name == "" {
			return "", ServiceProviderEntry{}, false
		}
		entry, ok := sc.Provider(name)
		return name, entry, ok
	}

	return "", ServiceProviderEntry{}, false
}

func requestedNativeProviderType(req ServiceExecuteRequest) string {
	if req.Provider != "" {
		return normalizeServiceProviderType(req.Provider)
	}
	switch req.Harness {
	case "", "agent":
		return ""
	default:
		return normalizeServiceProviderType(req.Harness)
	}
}

func (s *service) nativeProviderNotConfiguredError(req ServiceExecuteRequest, decision RouteDecision) string {
	wantedType := requestedNativeProviderType(req)
	if wantedType == "" {
		errMsg := "orphan model: " + decision.Model
		if decision.Model == "" {
			errMsg = "no provider configured for native harness"
		}
		return errMsg
	}
	available := s.availableProviderTypes()
	harness := decision.Harness
	if harness == "" {
		harness = "agent"
	}
	return fmt.Sprintf("harness %q: no configured provider matches type %q (available: %s)", harness, wantedType, available)
}

func (s *service) availableProviderTypes() string {
	sc := s.opts.ServiceConfig
	if sc == nil {
		return "[]"
	}
	var parts []string
	for _, name := range sc.ProviderNames() {
		entry, ok := sc.Provider(name)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", name, normalizeServiceProviderType(entry.Type)))
	}
	if len(parts) == 0 {
		return "[]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func buildNativeProvider(name string, entry ServiceProviderEntry) agentcore.Provider {
	switch normalizeServiceProviderType(entry.Type) {
	case "anthropic":
		return anthropic.New(anthropic.Config{
			BaseURL: entry.BaseURL,
			APIKey:  entry.APIKey,
			Model:   entry.Model,
		})
	case "lmstudio":
		return lmstudio.New(lmstudio.Config{
			BaseURL: entry.BaseURL,
			APIKey:  entry.APIKey,
			Model:   entry.Model,
		})
	case "openrouter":
		return openrouter.New(openrouter.Config{
			BaseURL: entry.BaseURL,
			APIKey:  entry.APIKey,
			Model:   entry.Model,
		})
	case "omlx":
		return omlx.New(omlx.Config{
			BaseURL: entry.BaseURL,
			APIKey:  entry.APIKey,
			Model:   entry.Model,
		})
	case "ollama":
		return ollama.New(ollama.Config{
			BaseURL: entry.BaseURL,
			APIKey:  entry.APIKey,
			Model:   entry.Model,
		})
	case "openai", "minimax", "qwen", "zai":
		return oaiProvider.New(oaiProvider.Config{
			BaseURL:        entry.BaseURL,
			APIKey:         entry.APIKey,
			Model:          entry.Model,
			ProviderName:   name,
			ProviderSystem: normalizeServiceProviderType(entry.Type),
		})
	default:
		return nil
	}
}
