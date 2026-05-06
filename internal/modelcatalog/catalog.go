package modelcatalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/fizeau/internal/reasoning"
	"github.com/DocumentDrivenDX/fizeau/internal/sampling"
)

// Surface identifies the consumer-specific naming surface for a model target.
type Surface string

const (
	SurfaceAgentOpenAI    Surface = "agent.openai"
	SurfaceAgentAnthropic Surface = "agent.anthropic"
	SurfaceCodex          Surface = "codex"
	SurfaceClaudeCode     Surface = "claude-code"
	SurfaceGemini         Surface = "gemini"
)

// ResolveOptions configures how model references are resolved.
type ResolveOptions struct {
	Surface         Surface
	AllowDeprecated bool
}

// Catalog resolves logical model references into concrete consumer-specific model IDs.
type Catalog struct {
	manifest       manifest
	manifestSrc    string
	aliasToID      map[string]string
	profileEntries map[string]profileEntry
}

// SurfacePolicy captures optional routing metadata for a resolved surface.
type SurfacePolicy struct {
	ReasoningDefault       reasoning.Reasoning
	PlacementOrder         []string
	MaxInputCostPerMTokUSD *float64
	FailurePolicy          string
}

// Metadata describes the loaded manifest.
type Metadata struct {
	ManifestSource  string
	ManifestVersion int
	CatalogVersion  string
}

// Profile describes one named catalog profile.
type Profile struct {
	Name                string
	Target              string
	CompatibilityTarget string
	MinPower            int
	MaxPower            int
	ProviderPreference  string
}

// Alias describes one catalog alias.
type Alias struct {
	Name        string
	Target      string
	Deprecated  bool
	Replacement string
}

// ResolvedTarget is the resolved output for a model reference.
type ResolvedTarget struct {
	Ref             string
	Profile         string
	Family          string
	CanonicalID     string
	ConcreteModel   string
	SurfacePolicy   SurfacePolicy
	Deprecated      bool
	Replacement     string
	CatalogVersion  string
	ManifestSource  string
	ManifestVersion int
	// Pricing (USD per 1M tokens, 0 = unknown/free)
	CostInputPerM      float64
	CostOutputPerM     float64
	CostCacheReadPerM  float64
	CostCacheWritePerM float64
	// Context
	ContextWindow int
	// Benchmarks
	SWEBenchVerified float64
	LiveCodeBench    float64
	BenchmarkAsOf    string
	// OpenRouter
	OpenRouterRefID string
}

// TierModel is one concrete model entry referenced by a catalog tier.
type TierModel struct {
	ID    string
	Entry ModelEntry
}

// ModelEligibility describes whether a catalog model can participate in
// unpinned automatic routing or only explicit model pins.
type ModelEligibility struct {
	Power        int
	ExactPinOnly bool
	AutoRoutable bool
}

// Metadata returns the loaded manifest metadata for inspection surfaces.
func (c *Catalog) Metadata() Metadata {
	return Metadata{
		ManifestSource:  c.manifestSrc,
		ManifestVersion: c.manifest.Version,
		CatalogVersion:  c.manifest.CatalogVersion,
	}
}

// Profiles returns all named profiles in deterministic order.
func (c *Catalog) Profiles() []Profile {
	names := make([]string, 0, len(c.profileEntries))
	for name := range c.profileEntries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Profile, 0, len(names))
	for _, name := range names {
		entry := c.profileEntries[name]
		compatTarget := profileCompatibilityTarget(entry)
		out = append(out, Profile{
			Name:                name,
			Target:              compatTarget,
			CompatibilityTarget: compatTarget,
			MinPower:            entry.MinPower,
			MaxPower:            entry.MaxPower,
			ProviderPreference:  normalizedProviderPreference(entry.ProviderPreference),
		})
	}
	return out
}

// Profile returns one named profile definition.
func (c *Catalog) Profile(name string) (Profile, bool) {
	entry, ok := c.profileEntries[strings.TrimSpace(name)]
	if !ok {
		return Profile{}, false
	}
	compatTarget := profileCompatibilityTarget(entry)
	return Profile{
		Name:                strings.TrimSpace(name),
		Target:              compatTarget,
		CompatibilityTarget: compatTarget,
		MinPower:            entry.MinPower,
		MaxPower:            entry.MaxPower,
		ProviderPreference:  normalizedProviderPreference(entry.ProviderPreference),
	}, true
}

// Aliases returns all target aliases in deterministic order.
func (c *Catalog) Aliases() []Alias {
	names := make([]string, 0, len(c.aliasToID))
	for name := range c.aliasToID {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Alias, 0, len(names))
	for _, name := range names {
		targetID := c.aliasToID[name]
		target := c.manifest.Targets[targetID]
		status := normalizedStatus(target.Status)
		out = append(out, Alias{
			Name:        name,
			Target:      targetID,
			Deprecated:  status != statusActive,
			Replacement: target.Replacement,
		})
	}
	return out
}

// UnknownReferenceError indicates that a reference is not known to the catalog.
type UnknownReferenceError struct {
	Ref string
}

func (e *UnknownReferenceError) Error() string {
	return fmt.Sprintf("modelcatalog: unknown reference %q", e.Ref)
}

// MissingSurfaceError indicates that a target cannot be projected to the requested surface.
type MissingSurfaceError struct {
	CanonicalID string
	Surface     Surface
}

func (e *MissingSurfaceError) Error() string {
	return fmt.Sprintf("modelcatalog: target %q has no mapping for surface %q", e.CanonicalID, e.Surface)
}

// DeprecatedTargetError indicates that a deprecated or stale target was resolved in strict mode.
type DeprecatedTargetError struct {
	CanonicalID string
	Status      string
	Replacement string
}

func (e *DeprecatedTargetError) Error() string {
	if e.Replacement == "" {
		return fmt.Sprintf("modelcatalog: target %q is %s", e.CanonicalID, e.Status)
	}
	return fmt.Sprintf("modelcatalog: target %q is %s; use %q", e.CanonicalID, e.Status, e.Replacement)
}

// UnknownTargetError indicates an internal invariant break where a referenced target is absent.
type UnknownTargetError struct {
	CanonicalID string
}

func (e *UnknownTargetError) Error() string {
	return fmt.Sprintf("modelcatalog: unknown target %q", e.CanonicalID)
}

// Current resolves a profile to its current target.
func (c *Catalog) Current(profile string, opts ResolveOptions) (ResolvedTarget, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return ResolvedTarget{}, &UnknownReferenceError{Ref: profile}
	}

	entry, ok := c.profileEntries[profile]
	if !ok {
		return ResolvedTarget{}, &UnknownReferenceError{Ref: profile}
	}

	return c.resolveTarget(profile, profile, profileCompatibilityTarget(entry), opts)
}

// Resolve resolves a profile, canonical target, or alias to a concrete model ID.
func (c *Catalog) Resolve(ref string, opts ResolveOptions) (ResolvedTarget, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ResolvedTarget{}, &UnknownReferenceError{Ref: ref}
	}

	if entry, ok := c.profileEntries[ref]; ok {
		return c.resolveTarget(ref, ref, profileCompatibilityTarget(entry), opts)
	}
	if _, ok := c.manifest.Targets[ref]; ok {
		return c.resolveTarget(ref, "", ref, opts)
	}
	if targetID, ok := c.aliasToID[ref]; ok {
		return c.resolveTarget(ref, "", targetID, opts)
	}

	return ResolvedTarget{}, &UnknownReferenceError{Ref: ref}
}

// AllConcreteModels returns a map from concrete model ID to catalog target ID
// for every active target that has a mapping for the given surface. The map is
// safe to use as a membership set for ranking discovered models.
// All candidate model IDs (not just the primary) are included.
// When multiple targets share the same concrete model ID, single-string surface
// entries take priority over candidates-list entries. Among entries of equal
// priority, the first target ID in lexicographic order wins.
func (c *Catalog) AllConcreteModels(surface Surface) map[string]string {
	// Sort target IDs for deterministic iteration.
	targetIDs := make([]string, 0, len(c.manifest.Targets))
	for targetID := range c.manifest.Targets {
		targetIDs = append(targetIDs, targetID)
	}
	sort.Strings(targetIDs)

	out := make(map[string]string)
	// First pass: single-string legacy surfaces (higher priority).
	for _, targetID := range targetIDs {
		entry := c.manifest.Targets[targetID]
		if normalizedStatus(entry.Status) != statusActive {
			continue
		}
		if sv, ok := entry.Surfaces[string(surface)]; ok && sv.model != "" {
			if sv.model != "" && out[sv.model] == "" {
				out[sv.model] = targetID
			}
		}
	}
	// Second pass: model-level candidates and legacy candidates-list entries
	// (lower priority, don't overwrite).
	for _, targetID := range targetIDs {
		entry := c.manifest.Targets[targetID]
		if normalizedStatus(entry.Status) != statusActive {
			continue
		}
		for _, concrete := range c.concreteModelsForSurface(entry, surface) {
			if concrete != "" && out[concrete] == "" {
				out[concrete] = targetID
			}
		}
		if sv, ok := entry.Surfaces[string(surface)]; ok && len(sv.candidates) > 0 {
			for _, candidate := range sv.candidates {
				if candidate != "" && out[candidate] == "" {
					out[candidate] = targetID
				}
			}
		}
	}
	return out
}

// SamplingProfile returns the catalog-defined sampling-parameter bundle for
// the given profile name (e.g., "code"). The second return value is false
// when the profile is not declared in the manifest. See ADR-007.
func (c *Catalog) SamplingProfile(name string) (sampling.Profile, bool) {
	p, ok := c.manifest.SamplingProfiles[name]
	return p, ok
}

// SamplingProfileNames returns the names of all sampling profiles declared
// in the manifest, sorted lexicographically. Used by `catalog show` and
// other ops surfaces to advertise what profiles are available.
func (c *Catalog) SamplingProfileNames() []string {
	if len(c.manifest.SamplingProfiles) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.manifest.SamplingProfiles))
	for name := range c.manifest.SamplingProfiles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ModelSamplingControl returns the SamplingControl string for the model ID,
// or "" when the model is not declared in the catalog or carries no
// explicit value (treated as the default "client_settable" by callers).
// Implements the sampling.CatalogLookup interface.
func (c *Catalog) ModelSamplingControl(modelID string) string {
	entry, ok := c.LookupModel(modelID)
	if !ok {
		return ""
	}
	return entry.SamplingControl
}

// LookupModel returns the ModelEntry for the given model ID from the top-level
// models: map (manifest v4+). The second return value is false if not found.
func (c *Catalog) LookupModel(id string) (ModelEntry, bool) {
	if entry, ok := c.manifest.Models[id]; ok {
		return entry, true
	}
	for _, entry := range c.manifest.Models {
		for _, concrete := range entry.Surfaces {
			if concrete == id {
				return entry, true
			}
		}
	}
	if entry, ok := c.lookupModelVariant(id); ok {
		return entry, true
	}
	return ModelEntry{}, false
}

// ModelEligibility returns power and automatic-routing eligibility for a
// catalog model ID or any declared provider surface ID. Unknown models return
// ok=false; known models with missing/zero power remain exact-pin-capable but
// are not auto-routable.
func (c *Catalog) ModelEligibility(id string) (ModelEligibility, bool) {
	entry, ok := c.LookupModel(id)
	if !ok {
		for modelID, candidate := range c.manifest.Models {
			if strings.EqualFold(modelID, id) {
				entry = candidate
				ok = true
				break
			}
			for _, surfaceID := range candidate.Surfaces {
				if strings.EqualFold(surfaceID, id) {
					entry = candidate
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
		if !ok {
			return ModelEligibility{}, false
		}
	}
	return ModelEligibility{
		Power:        entry.Power,
		ExactPinOnly: entry.ExactPinOnly,
		AutoRoutable: entry.AutoRoutable(),
	}, true
}

// AllModelsInTier returns the ordered model entries declared as candidates for
// a target tier. For older manifests, candidates are synthesized from surface
// mappings during load.
func (c *Catalog) AllModelsInTier(targetID string) []TierModel {
	target, ok := c.manifest.Targets[targetID]
	if !ok {
		return nil
	}
	ids := targetCandidateIDs(target)
	out := make([]TierModel, 0, len(ids))
	for _, id := range ids {
		entry, ok := c.manifest.Models[id]
		if !ok {
			continue
		}
		out = append(out, TierModel{ID: id, Entry: entry})
	}
	return out
}

// ContextWindowForModel returns the context window in tokens for the given
// concrete model ID, or 0 if the model is not in the catalog or has no
// context_window declared. Used as a fallback when the provider's live API
// does not expose its context window (e.g. LM Studio's /v1/models omits it).
// Matching is case-insensitive to accept both "qwen3.5-27b" and "Qwen3.5-27B".
func (c *Catalog) ContextWindowForModel(id string) int {
	if entry, ok := c.LookupModel(id); ok && entry.ContextWindow > 0 {
		return entry.ContextWindow
	}
	best := 0
	id = strings.TrimSpace(id)
	if id == "" {
		return 0
	}
	for modelID, entry := range c.manifest.Models {
		if strings.EqualFold(modelID, id) || sameModelVariant(id, modelID) {
			if entry.ContextWindow > best {
				best = entry.ContextWindow
			}
		}
		for _, concrete := range entry.Surfaces {
			if strings.EqualFold(concrete, id) || sameModelVariant(id, concrete) {
				if entry.ContextWindow > best {
					best = entry.ContextWindow
				}
			}
		}
	}
	if best > 0 {
		return best
	}
	return 0
}

// SupportsToolsForModel reports whether the given concrete model ID supports
// tool/function calling per the catalog. Returns true when the model is not
// in the catalog (caller assumes capable by default) and false only when the
// catalog explicitly marks the model with no_tools=true. Matching is
// case-insensitive to mirror ContextWindowForModel.
func (c *Catalog) SupportsToolsForModel(id string) bool {
	if c == nil {
		return true
	}
	if entry, ok := c.LookupModel(id); ok {
		return !entry.NoTools
	}
	return true
}

func (c *Catalog) lookupModelVariant(id string) (ModelEntry, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModelEntry{}, false
	}
	var (
		best     ModelEntry
		bestID   string
		bestSeen bool
	)
	for modelID, entry := range c.manifest.Models {
		matched := sameModelVariant(id, modelID)
		if !matched {
			for _, surfaceID := range entry.Surfaces {
				if sameModelVariant(id, surfaceID) {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		if !bestSeen || betterModelVariantMatch(entry, modelID, best, bestID) {
			best = entry
			bestID = modelID
			bestSeen = true
		}
	}
	if !bestSeen {
		return ModelEntry{}, false
	}
	return best, true
}

func betterModelVariantMatch(candidate ModelEntry, candidateID string, best ModelEntry, bestID string) bool {
	candidateStatus := normalizedStatus(candidate.Status)
	bestStatus := normalizedStatus(best.Status)
	if candidateStatus != bestStatus {
		return statusRank(candidateStatus) > statusRank(bestStatus)
	}
	if candidate.Power != best.Power {
		return candidate.Power > best.Power
	}
	if candidate.ContextWindow != best.ContextWindow {
		return candidate.ContextWindow > best.ContextWindow
	}
	if candidate.ExactPinOnly != best.ExactPinOnly {
		return !candidate.ExactPinOnly && best.ExactPinOnly
	}
	return candidateID < bestID
}

func statusRank(status string) int {
	switch normalizedStatus(status) {
	case statusActive:
		return 2
	case statusDeprecated:
		return 1
	case statusStale:
		return 0
	default:
		return -1
	}
}

func sameModelVariant(a, b string) bool {
	ca := catalogModelKey(a)
	cb := catalogModelKey(b)
	if ca == "" || cb == "" {
		return false
	}
	if ca == cb {
		return true
	}
	if sameNamedFamilyMajor(a, b) {
		return true
	}
	if len(ca) < 8 || len(cb) < 8 {
		return false
	}
	return strings.Contains(ca, cb) || strings.Contains(cb, ca)
}

func catalogModelKey(s string) string {
	s = strings.TrimSpace(s)
	if slash := strings.Index(s, "/"); slash > 0 {
		s = s[slash+1:]
	}
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sameNamedFamilyMajor(a, b string) bool {
	familyA, majorA := namedFamilyMajor(a)
	familyB, majorB := namedFamilyMajor(b)
	return familyA != "" && familyA == familyB && majorA > 0 && majorA == majorB
}

func namedFamilyMajor(s string) (string, int) {
	tokens := catalogModelTokens(s)
	if len(tokens) == 0 {
		return "", 0
	}
	family := ""
	for _, token := range tokens {
		switch token {
		case "opus", "sonnet", "haiku":
			family = token
		}
	}
	if family == "" {
		return "", 0
	}
	for _, token := range tokens {
		n, err := strconv.Atoi(token)
		if err == nil {
			return family, n
		}
	}
	return family, 0
}

func catalogModelTokens(s string) []string {
	s = strings.TrimSpace(s)
	if slash := strings.Index(s, "/"); slash > 0 {
		s = s[slash+1:]
	}
	s = strings.ToLower(s)
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return out
}

// CandidatesFor returns the ordered list of candidate concrete model IDs for
// the given surface and target key. For old-style single-string surfaces this
// returns a one-element slice. Returns nil if the target or surface is absent.
func (c *Catalog) CandidatesFor(surface Surface, targetKey string) []string {
	target, ok := c.manifest.Targets[targetKey]
	if !ok {
		return nil
	}
	if len(target.Candidates) > 0 {
		candidates := c.concreteModelsForSurface(target, surface)
		if len(candidates) == 0 {
			return nil
		}
		return candidates
	}
	sv, ok := target.Surfaces[string(surface)]
	if !ok {
		return nil
	}
	return sv.allCandidates()
}

// CatalogModelPricing holds per-million-token costs for a model as sourced from the catalog.
type CatalogModelPricing struct {
	InputPerMTok   float64
	OutputPerMTok  float64
	CacheReadPerM  float64
	CacheWritePerM float64
}

// AllModels returns all per-model entries from the top-level models: map
// (manifest v4+), keyed by model ID. Returns an empty map for older manifests.
func (c *Catalog) AllModels() map[string]ModelEntry {
	if len(c.manifest.Models) == 0 {
		return make(map[string]ModelEntry)
	}
	out := make(map[string]ModelEntry, len(c.manifest.Models))
	for id, e := range c.manifest.Models {
		out[id] = e
	}
	return out
}

// PricingFor returns pricing for all active concrete models across all surfaces.
// Per-model entries from the top-level models: map (v4+) take precedence over
// target-level pricing. Only models/targets with a positive input cost are
// included.
func (c *Catalog) PricingFor() map[string]CatalogModelPricing {
	result := make(map[string]CatalogModelPricing)

	for modelID, entry := range c.manifest.Models {
		input := entry.inputCostPerM()
		if input <= 0 {
			continue
		}
		result[modelID] = CatalogModelPricing{
			InputPerMTok:   input,
			OutputPerMTok:  entry.outputCostPerM(),
			CacheReadPerM:  entry.CostCacheReadPerM,
			CacheWritePerM: entry.CostCacheWritePerM,
		}
	}

	return result
}

func (c *Catalog) resolveTarget(ref, profile, targetID string, opts ResolveOptions) (ResolvedTarget, error) {
	if opts.Surface == "" {
		return ResolvedTarget{}, &MissingSurfaceError{CanonicalID: targetID, Surface: opts.Surface}
	}

	target, ok := c.manifest.Targets[targetID]
	if !ok {
		return ResolvedTarget{}, &UnknownTargetError{CanonicalID: targetID}
	}
	status := normalizedStatus(target.Status)
	deprecated := status != statusActive
	if deprecated && !opts.AllowDeprecated {
		return ResolvedTarget{}, &DeprecatedTargetError{
			CanonicalID: targetID,
			Status:      status,
			Replacement: target.Replacement,
		}
	}

	concreteModel, modelEntry, hasModelEntry := c.primaryConcreteModel(target, opts.Surface)
	if concreteModel == "" {
		return ResolvedTarget{}, &MissingSurfaceError{
			CanonicalID: targetID,
			Surface:     opts.Surface,
		}
	}
	policy := SurfacePolicy{}
	if target.SurfacePolicy != nil {
		if entry, ok := target.SurfacePolicy[string(opts.Surface)]; ok {
			policy = entry.toResolved()
		}
	}

	resolved := ResolvedTarget{
		Ref:                ref,
		Profile:            profile,
		Family:             target.Family,
		CanonicalID:        targetID,
		ConcreteModel:      concreteModel,
		SurfacePolicy:      policy,
		Deprecated:         deprecated,
		Replacement:        target.Replacement,
		CatalogVersion:     c.manifest.CatalogVersion,
		ManifestSource:     c.manifestSrc,
		ManifestVersion:    c.manifest.Version,
		CostInputPerM:      target.CostInputPerM,
		CostOutputPerM:     target.CostOutputPerM,
		CostCacheReadPerM:  target.CostCacheReadPerM,
		CostCacheWritePerM: target.CostCacheWritePerM,
		ContextWindow:      target.ContextWindow,
		SWEBenchVerified:   target.SWEBenchVerified,
		LiveCodeBench:      target.LiveCodeBench,
		BenchmarkAsOf:      target.BenchmarkAsOf,
		OpenRouterRefID:    target.OpenRouterRefID,
	}
	if hasModelEntry {
		if modelEntry.Family != "" {
			resolved.Family = modelEntry.Family
		}
		resolved.CostInputPerM = modelEntry.inputCostPerM()
		resolved.CostOutputPerM = modelEntry.outputCostPerM()
		resolved.CostCacheReadPerM = modelEntry.CostCacheReadPerM
		resolved.CostCacheWritePerM = modelEntry.CostCacheWritePerM
		resolved.ContextWindow = modelEntry.ContextWindow
		resolved.SWEBenchVerified = modelEntry.SWEBenchVerified
		resolved.LiveCodeBench = modelEntry.LiveCodeBench
		resolved.BenchmarkAsOf = modelEntry.BenchmarkAsOf
		resolved.OpenRouterRefID = modelEntry.openRouterID()
	}
	return resolved, nil
}

func (c *Catalog) primaryConcreteModel(target targetEntry, surface Surface) (string, ModelEntry, bool) {
	if sv, ok := target.Surfaces[string(surface)]; ok {
		modelID := sv.primaryModel()
		entry, hasEntry := c.manifest.Models[modelID]
		return modelID, entry, hasEntry
	}
	for _, modelID := range target.Candidates {
		entry, ok := c.manifest.Models[modelID]
		if !ok {
			continue
		}
		if concrete := entry.Surfaces[string(surface)]; concrete != "" {
			return concrete, entry, true
		}
	}
	return "", ModelEntry{}, false
}

func (c *Catalog) concreteModelsForSurface(target targetEntry, surface Surface) []string {
	if len(target.Candidates) == 0 {
		if sv, ok := target.Surfaces[string(surface)]; ok {
			return sv.allCandidates()
		}
		return nil
	}
	out := make([]string, 0, len(target.Candidates))
	for _, modelID := range target.Candidates {
		entry, ok := c.manifest.Models[modelID]
		if !ok {
			continue
		}
		if concrete := entry.Surfaces[string(surface)]; concrete != "" {
			out = append(out, concrete)
		}
	}
	return out
}

func targetCandidateIDs(target targetEntry) []string {
	if len(target.Candidates) > 0 {
		out := make([]string, len(target.Candidates))
		copy(out, target.Candidates)
		return out
	}
	seen := make(map[string]bool)
	var out []string
	keys := make([]string, 0, len(target.Surfaces))
	for surface := range target.Surfaces {
		keys = append(keys, surface)
	}
	sort.Strings(keys)
	for _, surface := range keys {
		for _, modelID := range target.Surfaces[surface].allCandidates() {
			if modelID != "" && !seen[modelID] {
				seen[modelID] = true
				out = append(out, modelID)
			}
		}
	}
	return out
}

func (m ModelEntry) inputCostPerM() float64 {
	if m.CostInputPerM != 0 {
		return m.CostInputPerM
	}
	return m.CostInputPerMTok
}

func (m ModelEntry) outputCostPerM() float64 {
	if m.CostOutputPerM != 0 {
		return m.CostOutputPerM
	}
	return m.CostOutputPerMTok
}

func (m ModelEntry) openRouterID() string {
	if m.OpenRouterID != "" {
		return m.OpenRouterID
	}
	return m.OpenRouterRefID
}

// AutoRoutable reports whether a model is eligible for unpinned automatic
// routing. Unknown-power and exact-pin-only entries remain usable by explicit
// model pin when live discovery confirms availability.
func (m ModelEntry) AutoRoutable() bool {
	return normalizedStatus(m.Status) == statusActive && m.Power > 0 && !m.ExactPinOnly
}
