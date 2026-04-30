package agentcli

import (
	"errors"
	"fmt"
	"io"
	"os"

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
	for _, name := range mountedSubcommands() {
		subcommandName := name
		cmd.AddCommand(&cobra.Command{
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
	cmd.SetIn(cfg.stdin)
	cmd.SetOut(cfg.stdout)
	cmd.SetErr(cfg.stderr)
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
