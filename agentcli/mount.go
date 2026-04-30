package agentcli

import (
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
		Use:                cfg.use,
		Short:              cfg.short,
		Long:               cfg.long,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
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
		},
	}
	cmd.SetIn(cfg.stdin)
	cmd.SetOut(cfg.stdout)
	cmd.SetErr(cfg.stderr)
	return cmd
}
