package agentcli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DocumentDrivenDX/agent/internal/productinfo"
	"github.com/spf13/cobra"
)

// MountOption customizes a mounted ddx-agent command tree.
type MountOption func(*mountConfig)

type mountConfig struct {
	use       string
	short     string
	long      string
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	version   string
	buildTime string
	gitCommit string
}

// ExitError reports a non-zero CLI runner exit without terminating the parent
// process.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s exited with code %d", productinfo.BinaryName, e.Code)
}

// ExitCode extracts a process-style exit code from errors returned by mounted
// command execution.
func ExitCode(err error) (int, bool) {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}

// WithUse overrides the Cobra command use/name string.
func WithUse(use string) MountOption {
	return func(cfg *mountConfig) {
		cfg.use = use
	}
}

// WithShort overrides the Cobra short help text.
func WithShort(short string) MountOption {
	return func(cfg *mountConfig) {
		cfg.short = short
	}
}

// WithLong overrides the Cobra long help text.
func WithLong(long string) MountOption {
	return func(cfg *mountConfig) {
		cfg.long = long
	}
}

// WithStdin injects stdin for mounted invocations.
func WithStdin(stdin io.Reader) MountOption {
	return func(cfg *mountConfig) {
		cfg.stdin = stdin
	}
}

// WithStdout injects stdout for mounted invocations.
func WithStdout(stdout io.Writer) MountOption {
	return func(cfg *mountConfig) {
		cfg.stdout = stdout
	}
}

// WithStderr injects stderr for mounted invocations.
func WithStderr(stderr io.Writer) MountOption {
	return func(cfg *mountConfig) {
		cfg.stderr = stderr
	}
}

// WithVersion injects version metadata for mounted invocations.
func WithVersion(version, buildTime, gitCommit string) MountOption {
	return func(cfg *mountConfig) {
		cfg.version = version
		cfg.buildTime = buildTime
		cfg.gitCommit = gitCommit
	}
}

// MountCLI returns a fresh, unattached Cobra root command. This wrapper
// intentionally delegates argument parsing to the existing runner until the
// native Cobra command tree lands.
func MountCLI(opts ...MountOption) *cobra.Command {
	cfg := mountConfig{
		use:    productinfo.BinaryName,
		short:  "Run the DDx agent CLI",
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	cmd := &cobra.Command{
		Use:           cfg.use,
		Short:         cfg.short,
		Long:          cfg.long,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMounted(cfg, legacyArgs(cmd, args...))
		},
	}
	addLegacyPersistentFlags(cmd)
	addMountedSubcommands(cmd, cfg)
	cmd.SetIn(cfg.stdin)
	cmd.SetOut(cfg.stdout)
	cmd.SetErr(cfg.stderr)
	return cmd
}

func addMountedSubcommands(root *cobra.Command, cfg mountConfig) {
	root.AddCommand(nativeLogCommand())
	root.AddCommand(nativeReplayCommand())
	root.AddCommand(nativeUsageCommand())
	root.AddCommand(nativeProvidersCommand())
	root.AddCommand(nativeModelsCommand())
	root.AddCommand(nativeCheckCommand())
	root.AddCommand(nativeCatalogCommand())
	root.AddCommand(nativeRouteStatusCommand())
	root.AddCommand(nativeCorpusCommand())
	root.AddCommand(nativeImportCommand())
	root.AddCommand(nativeUpdateCommand())
	for _, name := range []string{"version", "run"} {
		subcommandName := name
		root.AddCommand(&cobra.Command{
			Use:                subcommandName,
			SilenceUsage:       true,
			SilenceErrors:      true,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				legacy := legacyArgs(cmd, append([]string{subcommandName}, args...)...)
				return runMounted(cfg, legacy)
			},
		})
	}
}

func nativeLogCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "log [session-id]",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdLog(rootWorkDir(cmd), args))
		},
	}
}

func nativeReplayCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "replay <session-id>",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdReplay(rootWorkDir(cmd), args))
		},
	}
}

func nativeUsageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "usage",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdUsage(rootWorkDir(cmd), usageArgs(cmd, args)))
		},
	}
	cmd.Flags().String("since", "", "Time window: today, 7d, 30d, YYYY-MM-DD, or YYYY-MM-DD..YYYY-MM-DD")
	cmd.Flags().Bool("csv", false, "Output CSV")
	return cmd
}

func mountedSubcommands() []string {
	return []string{
		"log",
		"replay",
		"usage",
		"models",
		"check",
		"providers",
		"catalog",
		"corpus",
		"route-status",
		"import",
		"version",
		"update",
		"run",
	}
}

func nativeRouteStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "route-status",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdRouteStatus(rootWorkDir(cmd), routeStatusArgs(cmd, args)))
		},
	}
	cmd.Flags().String("profile", "", "Routing profile")
	cmd.Flags().String("harness", "", "Pin a specific harness")
	cmd.Flags().Bool("overrides", false, "Print override_class_breakdown over a time window")
	cmd.Flags().String("since", "", "Time window for --overrides mode")
	cmd.Flags().String("axis", "", "With --overrides, filter rows to this axis")
	return cmd
}

func nativeProvidersCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "providers",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdProviders(rootWorkDir(cmd), rootBool(cmd, "json")))
		},
	}
}

func nativeModelsCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "models",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdModels(rootWorkDir(cmd), rootBool(cmd, "json"), rootString(cmd, "provider"), args))
		},
	}
}

func nativeCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "check",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCheck(rootWorkDir(cmd), rootString(cmd, "provider"), args))
		},
	}
}

func nativeCorpusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "corpus",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCorpus(rootWorkDir(cmd), args))
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:           "validate",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCorpusValidate(rootWorkDir(cmd), args))
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:           "list",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCorpusList(rootWorkDir(cmd), corpusListArgs(cmd, args)))
		},
	})
	promote := &cobra.Command{
		Use:           "promote <bead-id>",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCorpusPromote(rootWorkDir(cmd), corpusPromoteArgs(cmd, args)))
		},
	}
	promote.Flags().String("capability", "", "Capability tag")
	promote.Flags().String("failure-mode", "", "Failure-mode tag")
	promote.Flags().String("difficulty", "", "easy|medium|hard")
	promote.Flags().String("prompt-kind", "", "Prompt kind")
	promote.Flags().String("notes", "", "Free-text notes")
	promote.Flags().String("base-rev", "", "Pre-change revision")
	promote.Flags().String("known-good-rev", "", "Resolution revision")
	promote.Flags().String("project-root", "", "Repository root")
	promote.Flags().String("promoted-by", "", "Promotion identity")
	promote.Flags().Bool("yes", false, "Skip confirmation")
	cmd.AddCommand(promote)
	return cmd
}

func nativeImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "import <pi|opencode>",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdImport(rootWorkDir(cmd), importArgs(cmd, args)))
		},
	}
	cmd.Flags().Bool("diff", false, "Show imported config diff")
	cmd.Flags().Bool("merge", false, "Merge providers into existing config")
	cmd.Flags().Bool("project", false, "Write project config")
	return cmd
}

func nativeUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "update",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdUpdate(updateArgs(cmd, args)))
		},
	}
	cmd.Flags().BoolP("check-only", "c", false, "Only check for an update")
	cmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	return cmd
}

func nativeCatalogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "catalog",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCatalog(rootWorkDir(cmd), args))
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:           "show",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCatalogShow(rootWorkDir(cmd), args))
		},
	})
	models := &cobra.Command{
		Use:           "models",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCatalogModels(rootWorkDir(cmd), catalogModelsArgs(cmd, args)))
		},
	}
	models.Flags().String("model", "", "Show details for a specific model ID")
	models.Flags().String("format", "", "Output format: json")
	cmd.AddCommand(models)

	observations := &cobra.Command{
		Use:           "observations",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCatalogObservations(rootWorkDir(cmd), catalogObservationArgs(cmd, args)))
		},
	}
	observations.Flags().String("format", "", "Output format: json")
	observations.Flags().String("provider", "", "Filter by provider")
	observations.Flags().String("model", "", "Filter by model")
	cmd.AddCommand(observations)

	check := &cobra.Command{
		Use:           "check",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCatalogCheck(rootWorkDir(cmd), catalogCheckArgs(cmd, args)))
		},
	}
	check.Flags().String("base-url", defaultCatalogBaseURL, "Published catalog base URL")
	check.Flags().String("channel", "stable", "Channel to inspect")
	check.Flags().String("version", "", "Exact catalog version to inspect")
	cmd.AddCommand(check)

	update := &cobra.Command{
		Use:           "update",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCatalogUpdate(rootWorkDir(cmd), catalogCheckArgs(cmd, args)))
		},
	}
	update.Flags().String("base-url", defaultCatalogBaseURL, "Published catalog base URL")
	update.Flags().String("channel", "stable", "Channel to install")
	update.Flags().String("version", "", "Exact catalog version to install")
	cmd.AddCommand(update)

	cmd.AddCommand(&cobra.Command{
		Use:           "update-pricing",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitError(cmdCatalogUpdatePricing(rootWorkDir(cmd), args))
		},
	})
	return cmd
}

func exitError(code int) error {
	if code == 0 {
		return nil
	}
	return &ExitError{Code: code}
}

func rootWorkDir(cmd *cobra.Command) string {
	return rootString(cmd, "work-dir")
}

func rootBool(cmd *cobra.Command, name string) bool {
	value, _ := cmd.Root().PersistentFlags().GetBool(name)
	return value
}

func rootString(cmd *cobra.Command, name string) string {
	value, _ := cmd.Root().PersistentFlags().GetString(name)
	return value
}

func changedFlagArgs(cmd *cobra.Command, names ...string) []string {
	out := make([]string, 0, len(names)*2)
	for _, name := range names {
		flag := cmd.Flags().Lookup(name)
		if flag != nil && flag.Changed {
			out = append(out, "--"+name, flag.Value.String())
		}
	}
	return out
}

func changedBoolFlagArgs(cmd *cobra.Command, names ...string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		flag := cmd.Flags().Lookup(name)
		if flag != nil && flag.Changed {
			if flag.Value.String() == "true" {
				out = append(out, "--"+name)
			} else {
				out = append(out, "--"+name+"="+flag.Value.String())
			}
		}
	}
	return out
}

func changedRootFlagArgs(cmd *cobra.Command, names ...string) []string {
	out := make([]string, 0, len(names)*2)
	flags := cmd.Root().PersistentFlags()
	for _, name := range names {
		flag := flags.Lookup(name)
		if flag != nil && flag.Changed {
			out = append(out, "--"+name, flag.Value.String())
		}
	}
	return out
}

func changedRootBoolFlagArgs(cmd *cobra.Command, names ...string) []string {
	out := make([]string, 0, len(names))
	flags := cmd.Root().PersistentFlags()
	for _, name := range names {
		flag := flags.Lookup(name)
		if flag != nil && flag.Changed {
			if flag.Value.String() == "true" {
				out = append(out, "--"+name)
			} else {
				out = append(out, "--"+name+"="+flag.Value.String())
			}
		}
	}
	return out
}

func usageArgs(cmd *cobra.Command, args []string) []string {
	out := changedRootBoolFlagArgs(cmd, "json")
	out = append(out, changedBoolFlagArgs(cmd, "csv")...)
	out = append(out, changedFlagArgs(cmd, "since")...)
	return append(out, args...)
}

func routeStatusArgs(cmd *cobra.Command, args []string) []string {
	out := changedRootBoolFlagArgs(cmd, "json")
	out = append(out, changedRootFlagArgs(cmd, "model", "model-ref", "provider", "min-power", "max-power")...)
	out = append(out, changedBoolFlagArgs(cmd, "overrides")...)
	out = append(out, changedFlagArgs(cmd, "profile", "harness", "since", "axis")...)
	return append(out, args...)
}

func corpusListArgs(cmd *cobra.Command, args []string) []string {
	out := changedRootBoolFlagArgs(cmd, "json")
	return append(out, args...)
}

func corpusPromoteArgs(cmd *cobra.Command, args []string) []string {
	out := changedBoolFlagArgs(cmd, "yes")
	out = append(out, changedFlagArgs(cmd,
		"capability",
		"failure-mode",
		"difficulty",
		"prompt-kind",
		"notes",
		"base-rev",
		"known-good-rev",
		"project-root",
		"promoted-by",
	)...)
	return append(out, args...)
}

func importArgs(cmd *cobra.Command, args []string) []string {
	out := append([]string{}, args...)
	return append(out, changedBoolFlagArgs(cmd, "diff", "merge", "project")...)
}

func updateArgs(cmd *cobra.Command, args []string) []string {
	out := changedBoolFlagArgs(cmd, "check-only", "force")
	return append(out, args...)
}

func catalogModelsArgs(cmd *cobra.Command, args []string) []string {
	out := changedFlagArgs(cmd, "model", "format")
	return append(out, args...)
}

func catalogObservationArgs(cmd *cobra.Command, args []string) []string {
	out := changedFlagArgs(cmd, "format", "provider", "model")
	return append(out, args...)
}

func catalogCheckArgs(cmd *cobra.Command, args []string) []string {
	out := changedFlagArgs(cmd, "base-url", "channel", "version")
	return append(out, args...)
}

// NeedsLegacyPassthrough reports whether argv should bypass Cobra traversal
// until the corresponding command path has native pflag definitions.
func NeedsLegacyPassthrough(args []string) bool {
	valueFlags := map[string]bool{
		"--backend":   true,
		"--max-power": true,
		"--max-iter":  true,
		"--min-power": true,
		"--model":     true,
		"--model-ref": true,
		"--p":         true,
		"-p":          true,
		"--preset":    true,
		"--provider":  true,
		"--reasoning": true,
		"--system":    true,
		"--work-dir":  true,
	}
	subcommands := map[string]bool{}
	for _, name := range mountedSubcommands() {
		subcommands[name] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if valueFlags[arg] && i+1 < len(args) {
				i++
			}
			continue
		}
		if arg == "run" {
			return false
		}
		switch arg {
		case "log", "replay", "usage", "providers", "models", "check", "route-status", "corpus", "import", "update":
			return false
		case "catalog":
			if i+1 < len(args) {
				switch args[i+1] {
				case "show", "models", "observations", "check", "update", "update-pricing":
					return false
				}
			}
			return true
		default:
			return subcommands[arg] || arg != ""
		}
	}
	return false
}

func addLegacyPersistentFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()
	flags.StringP("p", "p", "", "Prompt (use @file to read from file)")
	flags.Bool("json", false, "Output result as JSON")
	flags.String("provider", "", "Named provider from config")
	flags.String("backend", "", "Deprecated named backend pool from config")
	flags.String("model", "", "Model route key or explicit concrete model override")
	flags.String("model-ref", "", "Model catalog reference")
	flags.Bool("list-models", false, "List available models with routing metadata")
	flags.Int("min-power", 0, "Minimum catalog model power for automatic routing")
	flags.Int("max-power", 0, "Maximum catalog model power for automatic routing")
	flags.String("reasoning", "", "Reasoning control")
	flags.Bool("allow-deprecated-model", false, "Allow deprecated model catalog references")
	flags.Int("max-iter", 0, "Max iterations")
	flags.String("work-dir", "", "Working directory")
	flags.Bool("version", false, "Print version")
	flags.String("system", "", "System prompt")
	flags.String("preset", "", "System prompt preset")
}

func legacyArgs(cmd *cobra.Command, args ...string) []string {
	flags := cmd.Root().PersistentFlags()
	out := make([]string, 0, len(args)+flags.NFlag()*2)
	for _, name := range []string{
		"allow-deprecated-model",
		"json",
		"list-models",
		"version",
	} {
		if flag := flags.Lookup(name); flag != nil && flag.Changed {
			out = append(out, "--"+name)
		}
	}
	for _, name := range []string{
		"backend",
		"max-power",
		"max-iter",
		"min-power",
		"model",
		"model-ref",
		"p",
		"preset",
		"provider",
		"reasoning",
		"system",
		"work-dir",
	} {
		if flag := flags.Lookup(name); flag != nil && flag.Changed {
			out = append(out, "--"+name, flag.Value.String())
		}
	}
	out = append(out, args...)
	return out
}

func runMounted(cfg mountConfig, args []string) error {
	code := Run(Options{
		Args:      args,
		Stdin:     cfg.stdin,
		Stdout:    cfg.stdout,
		Stderr:    cfg.stderr,
		Version:   cfg.version,
		BuildTime: cfg.buildTime,
		GitCommit: cfg.gitCommit,
	})
	if code == 0 {
		return nil
	}
	return &ExitError{Code: code}
}
