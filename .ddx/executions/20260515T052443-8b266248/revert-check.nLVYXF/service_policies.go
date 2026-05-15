package fizeau

import (
	"context"
	"strings"

	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/routing"
)

func (s *service) ListPolicies(_ context.Context) ([]PolicyInfo, error) {
	cat, err := modelcatalog.Default()
	if err != nil {
		return nil, err
	}
	meta := cat.Metadata()
	policies := cat.Policies()
	out := make([]PolicyInfo, 0, len(policies))

	for _, policy := range policies {
		out = append(out, policyInfoFromCatalog(policy, meta))
	}

	return out, nil
}

func policyInfoFromCatalog(policy modelcatalog.Policy, meta modelcatalog.Metadata) PolicyInfo {
	return PolicyInfo{
		Name:            policy.Name,
		MinPower:        policy.MinPower,
		MaxPower:        policy.MaxPower,
		AllowLocal:      policy.AllowLocal,
		Require:         append([]string(nil), policy.Require...),
		CatalogVersion:  meta.CatalogVersion,
		ManifestSource:  meta.ManifestSource,
		ManifestVersion: meta.ManifestVersion,
	}
}

func policyForName(cat *modelcatalog.Catalog, name string) (modelcatalog.Policy, string, bool) {
	if cat == nil {
		return modelcatalog.Policy{}, "", false
	}
	name = strings.TrimSpace(name)
	if policy, ok := cat.Policy(name); ok {
		return policy, policy.Name, true
	}
	return modelcatalog.Policy{}, "", false
}

func providerPreferenceForPolicyName(name string) string {
	switch strings.TrimSpace(name) {
	case "air-gapped":
		return routing.ProviderPreferenceLocalOnly
	case "smart":
		return routing.ProviderPreferenceSubscriptionFirst
	case "default", "cheap":
		return routing.ProviderPreferenceLocalFirst
	default:
		return routing.ProviderPreferenceLocalFirst
	}
}
