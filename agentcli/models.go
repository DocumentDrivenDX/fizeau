package agentcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	agentConfig "github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/modelregistry"
	"github.com/easel/fizeau/internal/runtimesignals"
)

type modelsCommandOptions struct {
	JSON         bool
	Provider     string
	PowerMin     int
	PowerMax     int
	IncludeNoise bool
	Refresh      modelregistry.RefreshMode
	Ref          string
}

type modelDetail struct {
	CanonicalID       string                   `json:"canonical_id"`
	KnownModel        modelregistry.KnownModel `json:"known_model"`
	RawDiscoveryData  json.RawMessage          `json:"raw_discovery_data,omitempty"`
	CatalogEntry      *modelcatalog.ModelEntry `json:"catalog_entry,omitempty"`
	RuntimeSignal     *runtimesignals.Signal   `json:"runtime_signal,omitempty"`
	AutoRoutable      autoRoutableComposition  `json:"auto_routable"`
	SourceMeta        modelregistry.SourceMeta `json:"source_meta"`
	SnapshotGenerated time.Time                `json:"snapshot_generated_at"`
}

type autoRoutableComposition struct {
	AutoRoutable bool   `json:"auto_routable"`
	BlockedBy    string `json:"blocked_by,omitempty"`
	Provider     bool   `json:"provider"`
	Catalog      bool   `json:"catalog"`
	Status       bool   `json:"status"`
}

func cmdModels(workDir string, args []string) int {
	opts, err := parseModelsArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	snapshot, cfg, cat, cache, err := loadModelSnapshot(workDir, opts.Refresh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	if opts.Ref != "" {
		return cmdModelsDetail(snapshot, cfg, cat, cache, opts)
	}
	return cmdModelsList(snapshot, opts)
}

func parseModelsArgs(args []string) (modelsCommandOptions, error) {
	opts := modelsCommandOptions{Refresh: modelregistry.RefreshBackground}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--refresh":
			opts.Refresh = modelregistry.RefreshForce
		case arg == "--no-refresh":
			if opts.Refresh == modelregistry.RefreshForce {
				return opts, fmt.Errorf("--refresh and --no-refresh cannot be combined")
			}
			opts.Refresh = modelregistry.RefreshNone
		case arg == "--include-noise":
			opts.IncludeNoise = true
		case arg == "--provider":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--provider requires a value")
			}
			opts.Provider = args[i]
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.TrimPrefix(arg, "--provider=")
		case arg == "--power-min":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--power-min requires a value")
			}
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return opts, fmt.Errorf("--power-min: %w", err)
			}
			opts.PowerMin = value
		case strings.HasPrefix(arg, "--power-min="):
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--power-min="))
			if err != nil {
				return opts, fmt.Errorf("--power-min: %w", err)
			}
			opts.PowerMin = value
		case arg == "--power-max":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--power-max requires a value")
			}
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return opts, fmt.Errorf("--power-max: %w", err)
			}
			opts.PowerMax = value
		case strings.HasPrefix(arg, "--power-max="):
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--power-max="))
			if err != nil {
				return opts, fmt.Errorf("--power-max: %w", err)
			}
			opts.PowerMax = value
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown models flag %s", arg)
		default:
			if opts.Ref != "" {
				return opts, fmt.Errorf("models accepts at most one ref")
			}
			opts.Ref = arg
		}
	}
	return opts, nil
}

func loadModelSnapshot(workDir string, refresh modelregistry.RefreshMode) (modelregistry.ModelSnapshot, *agentConfig.Config, *modelcatalog.Catalog, *discoverycache.Cache, error) {
	cfg, err := agentConfig.Load(workDir)
	if err != nil {
		return modelregistry.ModelSnapshot{}, nil, nil, nil, err
	}
	cat, err := cfg.LoadModelCatalog()
	if err != nil {
		return modelregistry.ModelSnapshot{}, nil, nil, nil, err
	}
	root, err := modelCacheRoot()
	if err != nil {
		return modelregistry.ModelSnapshot{}, nil, nil, nil, err
	}
	cache := &discoverycache.Cache{Root: root}
	snapshot, err := modelregistry.AssembleWithOptions(context.Background(), cfg, cat, cache, modelregistry.AssembleOptions{Refresh: refresh})
	if err != nil {
		return modelregistry.ModelSnapshot{}, nil, nil, nil, err
	}
	return snapshot, cfg, cat, cache, nil
}

func modelCacheRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("FIZEAU_CACHE_DIR")); override != "" {
		return override, nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "fizeau"), nil
}

func cmdModelsList(snapshot modelregistry.ModelSnapshot, opts modelsCommandOptions) int {
	models := filterModelRows(snapshot.Models, opts)
	if opts.JSON {
		out := snapshot
		out.Models = models
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROVIDER\tMODEL\tFAMILY\tVERSION\tTIER\tPOWER\tCOST/M\tSTATUS\tCATALOG QUOTA\tRUNTIME QUOTA\tAUTO")
	for _, model := range models {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			model.Provider,
			model.ID,
			emptyDash(model.Family),
			formatVersion(model.Version),
			formatTier(model.Tier),
			formatModelPower(model.Power),
			formatModelCost(model.CostInputPerM, model.CostOutputPerM),
			model.Status,
			emptyDash(model.QuotaPool),
			formatRuntimeQuota(model.QuotaRemaining),
			formatAuto(model),
		)
	}
	_ = tw.Flush()
	return 0
}

func filterModelRows(models []modelregistry.KnownModel, opts modelsCommandOptions) []modelregistry.KnownModel {
	filtered := make([]modelregistry.KnownModel, 0, len(models))
	for _, model := range models {
		if opts.Provider != "" && model.Provider != opts.Provider {
			continue
		}
		if opts.PowerMin > 0 && model.Power < opts.PowerMin {
			continue
		}
		if opts.PowerMax > 0 && model.Power > opts.PowerMax {
			continue
		}
		filtered = append(filtered, model)
	}
	if !opts.IncludeNoise {
		filtered = suppressModelNoise(filtered)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Provider != filtered[j].Provider {
			return filtered[i].Provider < filtered[j].Provider
		}
		if filtered[i].Family != filtered[j].Family {
			return filtered[i].Family < filtered[j].Family
		}
		if cmp := compareVersions(filtered[i].Version, filtered[j].Version); cmp != 0 {
			return cmp > 0
		}
		return filtered[i].ID < filtered[j].ID
	})
	return filtered
}

func suppressModelNoise(models []modelregistry.KnownModel) []modelregistry.KnownModel {
	type groupKey struct {
		provider string
		family   string
		tier     modelcatalog.Tier
	}
	versionsByGroup := map[groupKey]map[string][]int{}
	for _, model := range models {
		if model.Power < 3 {
			continue
		}
		if model.Tier == modelcatalog.TierUnknown && model.Power == 0 {
			continue
		}
		key := groupKey{provider: model.Provider, family: model.Family, tier: model.Tier}
		if key.family == "" {
			continue
		}
		if versionsByGroup[key] == nil {
			versionsByGroup[key] = map[string][]int{}
		}
		versionsByGroup[key][formatVersion(model.Version)] = model.Version
	}

	allowedVersions := map[groupKey]map[string]bool{}
	for key, versions := range versionsByGroup {
		list := make([][]int, 0, len(versions))
		for _, version := range versions {
			list = append(list, version)
		}
		sort.Slice(list, func(i, j int) bool { return compareVersions(list[i], list[j]) > 0 })
		allowedVersions[key] = map[string]bool{}
		for i, version := range list {
			if i >= 2 {
				break
			}
			allowedVersions[key][formatVersion(version)] = true
		}
	}

	out := make([]modelregistry.KnownModel, 0, len(models))
	for _, model := range models {
		if model.Configured {
			out = append(out, model)
			continue
		}
		if model.Power < 3 {
			continue
		}
		if model.Tier == modelcatalog.TierUnknown && model.Power == 0 {
			continue
		}
		key := groupKey{provider: model.Provider, family: model.Family, tier: model.Tier}
		if allowedVersions[key] != nil && !allowedVersions[key][formatVersion(model.Version)] {
			continue
		}
		out = append(out, model)
	}
	if len(out) == 0 {
		return models
	}
	return out
}

func cmdModelsDetail(snapshot modelregistry.ModelSnapshot, cfg *agentConfig.Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache, opts modelsCommandOptions) int {
	model, err := resolveModel(snapshot.Models, opts.Ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	detail := buildModelDetail(snapshot, cfg, cat, cache, model)
	if opts.JSON {
		data, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	fmt.Printf("Canonical: %s\n", detail.CanonicalID)
	fmt.Printf("Identity: %s\n", modelIdentitySummary(detail.KnownModel))
	fmt.Printf("KnownModel: %+v\n", detail.KnownModel)
	fmt.Printf("ActualCashSpend: %t\n", detail.KnownModel.ActualCashSpend)
	fmt.Printf("EffectiveCost: %.4f source=%s\n", detail.KnownModel.EffectiveCost, labelOrUnknown(detail.KnownModel.EffectiveCostSource))
	fmt.Printf("HealthFreshness: at=%s source=%s\n", formatFreshness(detail.KnownModel.HealthFreshnessAt), labelOrUnknown(detail.KnownModel.HealthFreshnessSource))
	fmt.Printf("QuotaFreshness: at=%s source=%s\n", formatFreshness(detail.KnownModel.QuotaFreshnessAt), labelOrUnknown(detail.KnownModel.QuotaFreshnessSource))
	fmt.Printf("ModelDiscoveryFreshness: at=%s source=%s\n", formatFreshness(detail.KnownModel.DiscoveredAt), labelOrUnknown(string(detail.KnownModel.DiscoveredVia)))
	fmt.Printf("RuntimeQuotaRemaining: %s\n", formatRuntimeQuota(detail.KnownModel.QuotaRemaining))
	fmt.Printf("RecentP50Latency: %s\n", formatLatency(detail.KnownModel.RecentP50Latency))
	if detail.CatalogEntry != nil {
		fmt.Printf("CatalogEntry: %+v\n", *detail.CatalogEntry)
	} else {
		fmt.Println("CatalogEntry: -")
	}
	if len(detail.RawDiscoveryData) > 0 {
		fmt.Printf("RawDiscoveryData: %s\n", string(detail.RawDiscoveryData))
	} else {
		fmt.Println("RawDiscoveryData: -")
	}
	if detail.RuntimeSignal != nil {
		fmt.Printf("RuntimeSignal: %+v\n", *detail.RuntimeSignal)
	} else {
		fmt.Println("RuntimeSignal: -")
	}
	fmt.Printf("AutoRoutable: %t", detail.AutoRoutable.AutoRoutable)
	if detail.AutoRoutable.BlockedBy != "" {
		fmt.Printf(" blocked_by=%s", detail.AutoRoutable.BlockedBy)
	}
	fmt.Println()
	return 0
}

func resolveModel(models []modelregistry.KnownModel, ref string) (modelregistry.KnownModel, error) {
	ref = strings.TrimSpace(ref)
	if strings.Contains(ref, "/") {
		provider, id, ok := strings.Cut(ref, "/")
		if !ok || provider == "" || id == "" {
			return modelregistry.KnownModel{}, fmt.Errorf("invalid model %q", ref)
		}
		for _, model := range models {
			if model.Provider == provider && model.ID == id {
				return model, nil
			}
		}
		return modelregistry.KnownModel{}, fmt.Errorf("unknown model %q", ref)
	}

	var matches []modelregistry.KnownModel
	lower := strings.ToLower(ref)
	for _, model := range models {
		id := strings.ToLower(model.ID)
		if id == lower || strings.Contains(id, lower) {
			matches = append(matches, model)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return canonicalModelID(matches[i]) < canonicalModelID(matches[j]) })
	switch len(matches) {
	case 0:
		return modelregistry.KnownModel{}, fmt.Errorf("no model matched %q\n%s", ref, formatModelSuggestions(models, ref))
	case 1:
		return matches[0], nil
	default:
		return modelregistry.KnownModel{}, fmt.Errorf("ambiguous model %q\nCandidates:\n%s", ref, formatModelCandidates(matches))
	}
}

func buildModelDetail(snapshot modelregistry.ModelSnapshot, cfg *agentConfig.Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache, model modelregistry.KnownModel) modelDetail {
	detail := modelDetail{
		CanonicalID:       canonicalModelID(model),
		KnownModel:        model,
		RawDiscoveryData:  rawDiscoveryData(cache, model.Provider),
		AutoRoutable:      autoRoutableBreakdown(model, cfg),
		SourceMeta:        snapshot.Sources[model.Provider],
		SnapshotGenerated: snapshot.AsOf,
	}
	if cat != nil {
		if entry, ok := cat.LookupModel(model.ID); ok {
			detail.CatalogEntry = &entry
		}
	}
	if cache != nil {
		if sig, ok := runtimesignals.ReadCached(cache, model.Provider); ok {
			detail.RuntimeSignal = sig
		}
	}
	return detail
}

func rawDiscoveryData(cache *discoverycache.Cache, provider string) json.RawMessage {
	if cache == nil {
		return nil
	}
	result, err := cache.Read(discoverycache.Source{
		Tier:            "discovery",
		Name:            provider,
		TTL:             24 * time.Hour,
		RefreshDeadline: 10 * time.Second,
	})
	if err != nil || result.Data == nil {
		return nil
	}
	return json.RawMessage(result.Data)
}

func autoRoutableBreakdown(model modelregistry.KnownModel, cfg *agentConfig.Config) autoRoutableComposition {
	providerOK := true
	if cfg != nil {
		if pc, ok := cfg.Providers[model.Provider]; ok && pc.IncludeByDefault != nil {
			providerOK = *pc.IncludeByDefault
		}
	}
	catalogOK := model.Power > 0 && model.ExclusionReason != "catalog_unknown" && model.ExclusionReason != "catalog_not_auto_routable"
	statusOK := model.Status != modelregistry.StatusUnreachable && model.Status != modelregistry.StatusUnknown
	return autoRoutableComposition{
		AutoRoutable: model.AutoRoutable,
		BlockedBy:    model.ExclusionReason,
		Provider:     providerOK,
		Catalog:      catalogOK,
		Status:       statusOK,
	}
}

func cmdCachePrune(workDir string) int {
	cfg, err := agentConfig.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	root, err := modelCacheRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	cache := &discoverycache.Cache{Root: root}
	if err := cache.Prune(modelregistry.ActiveSources(cfg)); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	fmt.Println("cache pruned")
	return 0
}

func canonicalModelID(model modelregistry.KnownModel) string {
	return model.Provider + "/" + model.ID
}

func modelIdentitySummary(model modelregistry.KnownModel) string {
	parts := []string{canonicalModelID(model)}
	if model.Harness != "" {
		parts = append(parts, "harness="+model.Harness)
	}
	if model.ProviderType != "" {
		parts = append(parts, "provider_type="+model.ProviderType)
	}
	if model.EndpointName != "" {
		parts = append(parts, "endpoint_name="+model.EndpointName)
	}
	if model.EndpointBaseURL != "" {
		parts = append(parts, "endpoint_base_url="+model.EndpointBaseURL)
	}
	if model.ServerInstance != "" {
		parts = append(parts, "server_instance="+model.ServerInstance)
	}
	return strings.Join(parts, " ")
}

func formatModelCandidates(models []modelregistry.KnownModel) string {
	lines := make([]string, 0, len(models))
	for _, model := range models {
		lines = append(lines, "  "+canonicalModelID(model))
	}
	return strings.Join(lines, "\n")
}

func formatModelSuggestions(models []modelregistry.KnownModel, ref string) string {
	type scored struct {
		model modelregistry.KnownModel
		score int
	}
	scoredModels := make([]scored, 0, len(models))
	for _, model := range models {
		score := levenshtein(strings.ToLower(ref), strings.ToLower(model.ID))
		scoredModels = append(scoredModels, scored{model: model, score: score})
	}
	sort.Slice(scoredModels, func(i, j int) bool {
		if scoredModels[i].score != scoredModels[j].score {
			return scoredModels[i].score < scoredModels[j].score
		}
		return canonicalModelID(scoredModels[i].model) < canonicalModelID(scoredModels[j].model)
	})
	limit := len(scoredModels)
	if limit > 5 {
		limit = 5
	}
	if limit == 0 {
		return "Suggestions: -"
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		lines = append(lines, "  "+canonicalModelID(scoredModels[i].model))
	}
	return "Suggestions:\n" + strings.Join(lines, "\n")
}

func formatVersion(version []int) string {
	if len(version) == 0 {
		return "-"
	}
	parts := make([]string, len(version))
	for i, part := range version {
		parts[i] = strconv.Itoa(part)
	}
	return strings.Join(parts, ".")
}

func formatTier(tier modelcatalog.Tier) string {
	switch tier {
	case modelcatalog.TierSmart:
		return "smart"
	case modelcatalog.TierStandard:
		return "default"
	case modelcatalog.TierCheap:
		return "cheap"
	default:
		return "unknown"
	}
}

func formatModelPower(power int) string {
	if power <= 0 {
		return "-"
	}
	return strconv.Itoa(power)
}

func formatRuntimeQuota(quota *int) string {
	if quota == nil {
		return "-"
	}
	return strconv.Itoa(*quota)
}

func formatLatency(latency time.Duration) string {
	if latency <= 0 {
		return "-"
	}
	return latency.String()
}

func formatFreshness(at time.Time) string {
	if at.IsZero() {
		return "-"
	}
	return at.UTC().Format(time.RFC3339)
}

func formatModelCost(input, output float64) string {
	if input == 0 && output == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f/%.2f", input, output)
}

func formatAuto(model modelregistry.KnownModel) string {
	if model.AutoRoutable {
		return "yes"
	}
	if model.ExclusionReason != "" {
		return "no(" + model.ExclusionReason + ")"
	}
	return "no"
}

func compareVersions(a, b []int) int {
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
}

func levenshtein(a, b string) int {
	if a == "" {
		return len(b)
	}
	if b == "" {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			cur[j] = minInt(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func minInt(values ...int) int {
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
}
