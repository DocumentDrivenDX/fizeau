package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/easel/fizeau/internal/benchmark/profile"
	"github.com/easel/fizeau/internal/safefs"
	"gopkg.in/yaml.v3"
)

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type laneCloneConfig struct {
	workDir         string
	sweepPlan       string
	sourceLaneID    string
	laneID          string
	profileID       string
	recipes         []string
	aliases         []string
	provider        string
	model           string
	baseURL         string
	apiKeyEnv       string
	resourceGroup   string
	laneType        string
	modelFamily     string
	modelID         string
	quantLabel      string
	providerSurface string
	runtime         string
	hardwareLabel   string
	endpoint        string
	resolvedAt      string
	snapshot        string
	snapshotSuffix  string
	envOverrides    map[string]string
	sampling        map[string]string
	metadata        map[string]string
	dryRun          bool
}

type laneClonePlan struct {
	cfg               laneCloneConfig
	planPath          string
	sourceProfilePath string
	targetProfilePath string
	planBytes         []byte
	profileBytes      []byte
}

func cmdLanes(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s lanes <subcommand>\n\nSubcommands:\n  clone  Clone a benchmark lane/profile into the sweep plan\n", benchCommandName())
		return 2
	}
	switch args[0] {
	case "clone":
		return cmdLanesClone(args[1:])
	case "help", "-h", "--help":
		fmt.Fprintf(os.Stderr, "Usage: %s lanes clone [flags]\n", benchCommandName())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s lanes: unknown subcommand %q\n", benchCommandName(), args[0])
		return 2
	}
}

func cmdLanesClone(args []string) int {
	fs := flagSet("lanes clone")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	sweepPlan := fs.String("sweep-plan", "", "Path to sweep plan YAML (default: scripts/benchmark/terminalbench-2-1-sweep.yaml relative to --work-dir)")
	sourceLaneID := fs.String("from-lane", "", "Existing lane id to clone from")
	laneID := fs.String("lane-id", "", "New lane id to create")
	profileID := fs.String("profile-id", "", "New profile id / filename to create")
	recipesCSV := fs.String("recipes", "", "Optional comma-separated recipe ids to enroll the new lane into")
	aliasesCSV := fs.String("aliases", "", "Optional comma-separated short aliases to map to the new lane")
	provider := fs.String("provider", "", "Override FIZEAU_PROVIDER and profile.provider.type")
	model := fs.String("model", "", "Override FIZEAU_MODEL and profile.provider.model")
	baseURL := fs.String("base-url", "", "Override FIZEAU_BASE_URL and profile.provider.base_url")
	apiKeyEnv := fs.String("api-key-env", "", "Override FIZEAU_API_KEY_ENV and profile.provider.api_key_env")
	resourceGroup := fs.String("resource-group", "", "Override lane.resource_group")
	laneType := fs.String("lane-type", "", "Override lane.lane_type")
	modelFamily := fs.String("model-family", "", "Override lane.model_family")
	modelID := fs.String("model-id", "", "Override lane.model_id")
	quantLabel := fs.String("quant-label", "", "Override lane.quant_label")
	providerSurface := fs.String("provider-surface", "", "Override lane.provider_surface")
	runtime := fs.String("runtime", "", "Override lane.runtime")
	hardwareLabel := fs.String("hardware-label", "", "Override lane.hardware_label")
	endpoint := fs.String("endpoint", "", "Override lane.endpoint")
	resolvedAt := fs.String("resolved-at", "", "Override profile.versioning.resolved_at")
	snapshot := fs.String("snapshot", "", "Replace profile.versioning.snapshot")
	snapshotSuffix := fs.String("snapshot-suffix", "", "Append a suffix to profile.versioning.snapshot")
	dryRun := fs.Bool("dry-run", false, "Print planned file changes without writing")
	var envFlags multiFlag
	var samplingFlags multiFlag
	var metadataFlags multiFlag
	fs.Var(&envFlags, "env", "Repeatable KEY=VALUE override merged into lane.fizeau_env")
	fs.Var(&samplingFlags, "sampling", "Repeatable KEY=VALUE override for lane/profile sampling (temperature, reasoning, top_p, top_k, min_p)")
	fs.Var(&metadataFlags, "metadata", "Repeatable KEY=VALUE override inside the cloned profile's metadata block")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	envOverrides, err := parseKeyValueFlags(envFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s lanes clone: %v\n", benchCommandName(), err)
		return 2
	}
	sampling, err := parseSamplingFlags(samplingFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s lanes clone: %v\n", benchCommandName(), err)
		return 2
	}
	metadata, err := parseKeyValueFlags(metadataFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s lanes clone: %v\n", benchCommandName(), err)
		return 2
	}

	cfg := laneCloneConfig{
		workDir:         *workDir,
		sweepPlan:       *sweepPlan,
		sourceLaneID:    strings.TrimSpace(*sourceLaneID),
		laneID:          strings.TrimSpace(*laneID),
		profileID:       strings.TrimSpace(*profileID),
		recipes:         uniqueStringsPreserveOrder(splitCSV(*recipesCSV)),
		aliases:         uniqueStringsPreserveOrder(splitCSV(*aliasesCSV)),
		provider:        strings.TrimSpace(*provider),
		model:           strings.TrimSpace(*model),
		baseURL:         strings.TrimSpace(*baseURL),
		apiKeyEnv:       strings.TrimSpace(*apiKeyEnv),
		resourceGroup:   strings.TrimSpace(*resourceGroup),
		laneType:        strings.TrimSpace(*laneType),
		modelFamily:     strings.TrimSpace(*modelFamily),
		modelID:         strings.TrimSpace(*modelID),
		quantLabel:      strings.TrimSpace(*quantLabel),
		providerSurface: strings.TrimSpace(*providerSurface),
		runtime:         strings.TrimSpace(*runtime),
		hardwareLabel:   strings.TrimSpace(*hardwareLabel),
		endpoint:        strings.TrimSpace(*endpoint),
		resolvedAt:      strings.TrimSpace(*resolvedAt),
		snapshot:        *snapshot,
		snapshotSuffix:  *snapshotSuffix,
		envOverrides:    envOverrides,
		sampling:        sampling,
		metadata:        metadata,
		dryRun:          *dryRun,
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%s lanes clone: %v\n", benchCommandName(), err)
		return 2
	}

	plan, err := planLaneClone(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s lanes clone: %v\n", benchCommandName(), err)
		return 1
	}

	if cfg.dryRun {
		printLaneCloneDryRun(plan)
		return 0
	}

	if err := writeTextAtomic(plan.planPath, plan.planBytes); err != nil {
		fmt.Fprintf(os.Stderr, "%s lanes clone: write %s: %v\n", benchCommandName(), plan.planPath, err)
		return 1
	}
	if err := writeTextAtomic(plan.targetProfilePath, plan.profileBytes); err != nil {
		fmt.Fprintf(os.Stderr, "%s lanes clone: write %s: %v\n", benchCommandName(), plan.targetProfilePath, err)
		return 1
	}

	fmt.Printf("Updated %s\n", plan.planPath)
	fmt.Printf("Created %s\n", plan.targetProfilePath)
	return 0
}

func (cfg laneCloneConfig) Validate() error {
	if cfg.sourceLaneID == "" {
		return fmt.Errorf("--from-lane is required")
	}
	if cfg.laneID == "" {
		return fmt.Errorf("--lane-id is required")
	}
	if cfg.profileID == "" {
		return fmt.Errorf("--profile-id is required")
	}
	if cfg.sourceLaneID == cfg.laneID {
		return fmt.Errorf("--lane-id must differ from --from-lane")
	}
	if cfg.snapshot != "" && cfg.snapshotSuffix != "" {
		return fmt.Errorf("--snapshot and --snapshot-suffix are mutually exclusive")
	}
	if cfg.provider != "" && !isValidBenchmarkProviderType(cfg.provider) {
		return fmt.Errorf("--provider %q is not a supported benchmark provider type", cfg.provider)
	}
	return nil
}

func planLaneClone(cfg laneCloneConfig) (*laneClonePlan, error) {
	wd := resolveWorkDir(cfg.workDir)
	planPath := cfg.sweepPlan
	if planPath == "" {
		planPath = filepath.Join(wd, defaultSweepPlanPath)
	} else if !filepath.IsAbs(planPath) {
		planPath = filepath.Join(wd, planPath)
	}

	planDoc, err := loadYAMLDocument(planPath)
	if err != nil {
		return nil, fmt.Errorf("load sweep plan %s: %w", planPath, err)
	}
	planStruct, err := loadSweepPlan(planPath)
	if err != nil {
		return nil, fmt.Errorf("validate sweep plan %s: %w", planPath, err)
	}

	laneByID := sweepLaneMap(planStruct)
	sourceLane, ok := laneByID[cfg.sourceLaneID]
	if !ok {
		return nil, fmt.Errorf("source lane %q not found in %s", cfg.sourceLaneID, planPath)
	}
	if _, exists := laneByID[cfg.laneID]; exists {
		return nil, fmt.Errorf("lane %q already exists in %s", cfg.laneID, planPath)
	}

	if cfg.resourceGroup != "" {
		if _, ok := sweepRGMap(planStruct)[cfg.resourceGroup]; !ok {
			return nil, fmt.Errorf("resource group %q not found in %s", cfg.resourceGroup, planPath)
		}
	}
	recipeIDs := cfg.recipes
	if len(recipeIDs) > 0 {
		recipeByID := sweepRecipeMap(planStruct)
		for _, recipeID := range recipeIDs {
			if _, ok := recipeByID[recipeID]; !ok {
				return nil, fmt.Errorf("recipe %q not found in %s", recipeID, planPath)
			}
		}
	}
	if len(cfg.aliases) > 0 {
		for _, alias := range cfg.aliases {
			if existing := strings.TrimSpace(planStruct.LaneAliases[alias]); existing != "" && existing != cfg.laneID {
				return nil, fmt.Errorf("alias %q already maps to lane %q", alias, existing)
			}
		}
	}

	profilesDir := filepath.Join(filepath.Dir(planPath), "profiles")
	sourceProfilePath := filepath.Join(profilesDir, sourceLane.ProfileID+".yaml")
	targetProfilePath := filepath.Join(profilesDir, cfg.profileID+".yaml")
	if _, err := os.Stat(targetProfilePath); err == nil {
		return nil, fmt.Errorf("target profile %q already exists at %s", cfg.profileID, targetProfilePath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat target profile %s: %w", targetProfilePath, err)
	}

	sourceProfileDoc, err := loadYAMLDocument(sourceProfilePath)
	if err != nil {
		return nil, fmt.Errorf("load source profile %s: %w", sourceProfilePath, err)
	}
	rootMap, err := yamlDocumentRootMap(planDoc)
	if err != nil {
		return nil, err
	}
	lanesSeq, err := yamlMapSequence(rootMap, "lanes")
	if err != nil {
		return nil, err
	}
	sourceLaneNode := yamlFindSequenceMapByID(lanesSeq, cfg.sourceLaneID)
	if sourceLaneNode == nil {
		return nil, fmt.Errorf("source lane %q not found in YAML plan", cfg.sourceLaneID)
	}
	clonedLane := cloneYAMLNode(sourceLaneNode)
	applyLaneCloneOverrides(clonedLane, cfg, sourceLane)
	lanesSeq.Content = append(lanesSeq.Content, clonedLane)

	if len(recipeIDs) > 0 {
		recipesSeq, err := yamlMapSequence(rootMap, "recipes")
		if err != nil {
			return nil, err
		}
		for _, recipeID := range recipeIDs {
			recipeNode := yamlFindSequenceMapByID(recipesSeq, recipeID)
			if recipeNode == nil {
				return nil, fmt.Errorf("recipe %q not found in YAML plan", recipeID)
			}
			recipeLanes, err := yamlMapSequence(recipeNode, "lanes")
			if err != nil {
				return nil, fmt.Errorf("recipe %q lanes: %w", recipeID, err)
			}
			yamlSequenceAppendUniqueString(recipeLanes, cfg.laneID)
		}
	}

	if len(cfg.aliases) > 0 {
		aliasMap := yamlMapLookup(rootMap, "lane_aliases")
		if aliasMap == nil {
			aliasMap = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			yamlMapSet(rootMap, "lane_aliases", aliasMap)
		}
		if aliasMap.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("lane_aliases must be a YAML mapping")
		}
		for _, alias := range cfg.aliases {
			yamlMapSet(aliasMap, alias, yamlStringNode(cfg.laneID))
		}
	}

	clonedProfileDoc := cloneYAMLNode(sourceProfileDoc)
	profileRoot, err := yamlDocumentRootMap(clonedProfileDoc)
	if err != nil {
		return nil, err
	}
	applyProfileCloneOverrides(profileRoot, cfg)

	planBytes, err := marshalYAMLDocument(planDoc)
	if err != nil {
		return nil, fmt.Errorf("render sweep plan: %w", err)
	}
	profileBytes, err := marshalYAMLDocument(clonedProfileDoc)
	if err != nil {
		return nil, fmt.Errorf("render profile: %w", err)
	}
	if err := validateLaneCloneRender(planPath, planBytes, cfg.profileID, profileBytes); err != nil {
		return nil, err
	}

	return &laneClonePlan{
		cfg:               cfg,
		planPath:          planPath,
		sourceProfilePath: sourceProfilePath,
		targetProfilePath: targetProfilePath,
		planBytes:         planBytes,
		profileBytes:      profileBytes,
	}, nil
}

func applyLaneCloneOverrides(node *yaml.Node, cfg laneCloneConfig, sourceLane *sweepLane) {
	yamlMapSet(node, "id", yamlStringNode(cfg.laneID))
	yamlMapSet(node, "profile_id", yamlStringNode(cfg.profileID))
	if cfg.resourceGroup != "" {
		yamlMapSet(node, "resource_group", yamlStringNode(cfg.resourceGroup))
	}
	if cfg.laneType != "" {
		yamlMapSet(node, "lane_type", yamlStringNode(cfg.laneType))
	}
	if cfg.modelFamily != "" {
		yamlMapSet(node, "model_family", yamlStringNode(cfg.modelFamily))
	}
	if cfg.modelID != "" {
		yamlMapSet(node, "model_id", yamlStringNode(cfg.modelID))
	}
	if cfg.quantLabel != "" {
		yamlMapSet(node, "quant_label", yamlStringNode(cfg.quantLabel))
	}
	if cfg.providerSurface != "" {
		yamlMapSet(node, "provider_surface", yamlStringNode(cfg.providerSurface))
	}
	if cfg.runtime != "" {
		yamlMapSet(node, "runtime", yamlStringNode(cfg.runtime))
	}
	if cfg.hardwareLabel != "" {
		yamlMapSet(node, "hardware_label", yamlStringNode(cfg.hardwareLabel))
	}
	if cfg.endpoint != "" {
		yamlMapSet(node, "endpoint", yamlStringNode(cfg.endpoint))
	}

	envMap := yamlMapEnsureMapping(node, "fizeau_env")
	if cfg.provider != "" {
		yamlMapSet(envMap, "FIZEAU_PROVIDER", yamlStringNode(cfg.provider))
	}
	if cfg.model != "" {
		yamlMapSet(envMap, "FIZEAU_MODEL", yamlStringNode(cfg.model))
	}
	if cfg.baseURL != "" {
		yamlMapSet(envMap, "FIZEAU_BASE_URL", yamlStringNode(cfg.baseURL))
	}
	if cfg.apiKeyEnv != "" {
		yamlMapSet(envMap, "FIZEAU_API_KEY_ENV", yamlStringNode(cfg.apiKeyEnv))
	}
	for _, key := range sortedKeys(cfg.envOverrides) {
		yamlMapSet(envMap, key, yamlStringNode(cfg.envOverrides[key]))
	}

	samplingMap := yamlMapEnsureMapping(node, "sampling")
	for _, key := range sortedKeys(cfg.sampling) {
		yamlMapSet(samplingMap, key, samplingScalarNode(key, cfg.sampling[key]))
	}

	// Preserve the source lane's effective profile id for recipe-sourcing tests.
	_ = sourceLane
}

func applyProfileCloneOverrides(node *yaml.Node, cfg laneCloneConfig) {
	yamlMapSet(node, "id", yamlStringNode(cfg.profileID))

	providerMap := yamlMapEnsureMapping(node, "provider")
	if cfg.provider != "" {
		yamlMapSet(providerMap, "type", yamlStringNode(cfg.provider))
	}
	if cfg.model != "" {
		yamlMapSet(providerMap, "model", yamlStringNode(cfg.model))
	}
	if cfg.baseURL != "" {
		yamlMapSet(providerMap, "base_url", yamlStringNode(cfg.baseURL))
	}
	if cfg.apiKeyEnv != "" {
		yamlMapSet(providerMap, "api_key_env", yamlStringNode(cfg.apiKeyEnv))
	}

	samplingMap := yamlMapEnsureMapping(node, "sampling")
	for _, key := range sortedKeys(cfg.sampling) {
		yamlMapSet(samplingMap, key, samplingScalarNode(key, cfg.sampling[key]))
	}

	if len(cfg.metadata) > 0 {
		metadataMap := yamlMapEnsureMapping(node, "metadata")
		for _, key := range sortedKeys(cfg.metadata) {
			yamlMapSet(metadataMap, key, yamlStringNode(cfg.metadata[key]))
		}
	}

	versioningMap := yamlMapEnsureMapping(node, "versioning")
	if cfg.resolvedAt != "" {
		yamlMapSet(versioningMap, "resolved_at", yamlStringNode(cfg.resolvedAt))
	}
	if cfg.snapshot != "" {
		yamlMapSet(versioningMap, "snapshot", yamlStringNode(cfg.snapshot))
	} else if cfg.snapshotSuffix != "" {
		snapshot := yamlMapLookup(versioningMap, "snapshot")
		if snapshot == nil {
			yamlMapSet(versioningMap, "snapshot", yamlStringNode(cfg.snapshotSuffix))
		} else {
			snapshot.Value += cfg.snapshotSuffix
		}
	}
}

func validateLaneCloneRender(planPath string, planBytes []byte, profileID string, profileBytes []byte) error {
	validateRoot, err := os.MkdirTemp("", "fiz-bench-lane-clone-*")
	if err != nil {
		return fmt.Errorf("create validation dir: %w", err)
	}
	defer os.RemoveAll(validateRoot)

	relPlan := defaultSweepPlanPath
	validatePlanPath := filepath.Join(validateRoot, relPlan)
	validateProfilesDir := filepath.Join(filepath.Dir(validatePlanPath), "profiles")
	if err := os.MkdirAll(validateProfilesDir, 0o750); err != nil {
		return fmt.Errorf("create validation profiles dir: %w", err)
	}
	origProfilesDir := filepath.Join(filepath.Dir(planPath), "profiles")
	entries, err := os.ReadDir(origProfilesDir)
	if err != nil {
		return fmt.Errorf("read source profiles dir %s: %w", origProfilesDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		src := filepath.Join(origProfilesDir, entry.Name())
		dst := filepath.Join(validateProfilesDir, entry.Name())
		if err := copyTextFile(src, dst); err != nil {
			return fmt.Errorf("copy profile %s: %w", entry.Name(), err)
		}
	}
	if err := writeTextAtomic(validatePlanPath, planBytes); err != nil {
		return fmt.Errorf("write validation sweep plan: %w", err)
	}
	if err := writeTextAtomic(filepath.Join(validateProfilesDir, profileID+".yaml"), profileBytes); err != nil {
		return fmt.Errorf("write validation profile: %w", err)
	}
	if _, err := loadSweepPlan(validatePlanPath); err != nil {
		return fmt.Errorf("validate rendered clone: %w", err)
	}
	return nil
}

func printLaneCloneDryRun(plan *laneClonePlan) {
	fmt.Printf("Dry run: no files written.\n")
	fmt.Printf("  Sweep plan: %s\n", plan.planPath)
	fmt.Printf("    add lane %s cloned from %s\n", plan.cfg.laneID, plan.cfg.sourceLaneID)
	if len(plan.cfg.recipes) > 0 {
		fmt.Printf("    enroll recipes: %s\n", strings.Join(plan.cfg.recipes, ", "))
	}
	if len(plan.cfg.aliases) > 0 {
		fmt.Printf("    add aliases: %s\n", strings.Join(plan.cfg.aliases, ", "))
	}
	fmt.Printf("  Profile: %s\n", plan.targetProfilePath)
	fmt.Printf("    clone source profile %s\n", plan.sourceProfilePath)
	if len(plan.cfg.envOverrides) > 0 {
		fmt.Printf("    env overrides: %s\n", strings.Join(formatMapPairs(plan.cfg.envOverrides), ", "))
	}
	if len(plan.cfg.sampling) > 0 {
		fmt.Printf("    sampling overrides: %s\n", strings.Join(formatMapPairs(plan.cfg.sampling), ", "))
	}
	if len(plan.cfg.metadata) > 0 {
		fmt.Printf("    metadata overrides: %s\n", strings.Join(formatMapPairs(plan.cfg.metadata), ", "))
	}
}

func parseKeyValueFlags(values []string) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for _, raw := range values {
		key, val, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", raw)
		}
		out[key] = val
	}
	return out, nil
}

func parseSamplingFlags(values []string) (map[string]string, error) {
	out, err := parseKeyValueFlags(values)
	if err != nil {
		return nil, err
	}
	for key, value := range out {
		switch key {
		case "temperature", "top_p", "min_p":
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				return nil, fmt.Errorf("sampling %s=%q must be a float: %w", key, value, err)
			}
		case "top_k":
			if _, err := strconv.Atoi(value); err != nil {
				return nil, fmt.Errorf("sampling %s=%q must be an int: %w", key, value, err)
			}
		case "reasoning":
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("sampling reasoning must not be empty")
			}
		default:
			return nil, fmt.Errorf("unsupported sampling key %q", key)
		}
	}
	return out, nil
}

func samplingScalarNode(key, value string) *yaml.Node {
	switch key {
	case "temperature", "top_p", "min_p":
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: value}
	case "top_k":
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value}
	default:
		return yamlStringNode(value)
	}
}

func uniqueStringsPreserveOrder(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func isValidBenchmarkProviderType(value string) bool {
	switch profile.ProviderType(value) {
	case profile.ProviderAnthropic,
		profile.ProviderOpenAI,
		profile.ProviderOpenAICompat,
		profile.ProviderOpenRouter,
		profile.ProviderOMLX,
		profile.ProviderLMStudio,
		profile.ProviderOllama,
		profile.ProviderGoogle,
		profile.ProviderRapidMLX,
		profile.ProviderVLLM,
		profile.ProviderLlamaServer,
		profile.ProviderDS4,
		profile.ProviderLucebox:
		return true
	default:
		return false
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatMapPairs(values map[string]string) []string {
	keys := sortedKeys(values)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func loadYAMLDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-controlled benchmark path
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func marshalYAMLDocument(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	cloned := *node
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			cloned.Content[i] = cloneYAMLNode(child)
		}
	}
	return &cloned
}

func yamlDocumentRootMap(doc *yaml.Node) (*yaml.Node, error) {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("expected YAML document root")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected YAML mapping root")
	}
	return root, nil
}

func yamlMapLookup(mapNode *yaml.Node, key string) *yaml.Node {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}

func yamlMapSequence(mapNode *yaml.Node, key string) (*yaml.Node, error) {
	value := yamlMapLookup(mapNode, key)
	if value == nil {
		return nil, fmt.Errorf("missing YAML key %q", key)
	}
	if value.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("YAML key %q must be a sequence", key)
	}
	return value, nil
}

func yamlMapEnsureMapping(mapNode *yaml.Node, key string) *yaml.Node {
	if value := yamlMapLookup(mapNode, key); value != nil && value.Kind == yaml.MappingNode {
		return value
	}
	value := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	yamlMapSet(mapNode, key, value)
	return value
}

func yamlMapSet(mapNode *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			mapNode.Content[i+1] = value
			return
		}
	}
	mapNode.Content = append(mapNode.Content, yamlStringNode(key), value)
}

func yamlFindSequenceMapByID(seq *yaml.Node, id string) *yaml.Node {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if idNode := yamlMapLookup(item, "id"); idNode != nil && idNode.Value == id {
			return item
		}
	}
	return nil
}

func yamlSequenceAppendUniqueString(seq *yaml.Node, value string) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range seq.Content {
		if item.Kind == yaml.ScalarNode && item.Value == value {
			return
		}
	}
	seq.Content = append(seq.Content, yamlStringNode(value))
}

func yamlStringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func writeTextAtomic(path string, data []byte) error {
	if err := safefs.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return safefs.WriteFileAtomic(path, data, 0o600)
}

func copyTextFile(src, dst string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- source is a repo-owned benchmark file
	if err != nil {
		return err
	}
	return writeTextAtomic(dst, data)
}
