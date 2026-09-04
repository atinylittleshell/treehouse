package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/kunchenguid/treehouse/internal/deadline"
	"github.com/kunchenguid/treehouse/internal/updater"
	"github.com/spf13/cobra"
)

var version = "dev"

// rootFlag holds the value of the persistent --root flag. It overrides both the
// TREEHOUSE_ROOT environment variable and the configured root; see
// config.ResolveRoot for the full precedence.
var rootFlag string

// timeoutFlag holds the value of the persistent --timeout flag, which bounds
// how long a command may block waiting on the pool lock or on a git/jj
// subprocess. See resolveTimeout for the full precedence.
var timeoutFlag time.Duration

// resolveTimeout picks the command timeout: an explicit --timeout wins, then
// TREEHOUSE_TIMEOUT, then deadline.Default. A zero or negative value from
// either source disables the bound. An unparseable TREEHOUSE_TIMEOUT is
// reported and ignored rather than failing the command, so a typo in the
// environment cannot make treehouse unusable.
func resolveTimeout(cmd *cobra.Command) time.Duration {
	// An explicit --timeout wins even at 0, which is how a caller asks to wait
	// forever; Changed is what separates that from an untouched default.
	if cmd.Flags().Changed("timeout") {
		return timeoutFlag
	}
	if raw := os.Getenv("TREEHOUSE_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "🌳 Warning: ignoring TREEHOUSE_TIMEOUT=%q: %v\n", raw, err)
		} else {
			return d
		}
	}
	return deadline.Default
}

func SetVersion(v string) {
	version = v
	rootCmd.Version = v
}

var rootCmd = &cobra.Command{
	Use:   "treehouse",
	Short: "Manage a pool of git worktrees for parallel AI agent workflows",
	Long: `Treehouse maintains a pool of reusable, pre-warmed git worktrees
so that multiple AI coding agents can work on the same repo in parallel.`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return getRunE(cmd, args)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Bound every blocking wait this process makes. Without a bound a
		// stalled origin or a stalled lock holder parks the command forever,
		// and no timeout the caller applies can reach it.
		deadline.Set(resolveTimeout(cmd))

		// Skip update check for dev builds, the update command itself,
		// or when explicitly suppressed via env var.
		if version == "dev" || os.Getenv("TREEHOUSE_NO_UPDATE_CHECK") == "1" {
			return
		}
		if cmd.Name() == "update" {
			return
		}

		// Show cached update notice from a previous check
		if result := updater.ReadCache(version); result != nil && result.UpdateAvailable {
			yellow := color.New(color.FgYellow)
			yellow.Fprintf(os.Stderr, "A new version of treehouse is available: %s → %s\n", version, result.LatestVersion)
			yellow.Fprintln(os.Stderr, "Run \"treehouse update\" to update")
			fmt.Fprintln(os.Stderr)
		}

		// Spawn background check if cache is stale
		if updater.IsCacheStale(version) {
			_ = updater.SpawnBackgroundCheck(version)
		}
	},
}

func init() {
	rootCmd.SetVersionTemplate(`{{.Version}}` + "\n")
	rootCmd.PersistentFlags().StringVar(&rootFlag, "root", "",
		"Worktree root directory, overriding TREEHOUSE_ROOT and config; relative paths (e.g. \".\" for an in-project pool) resolve from the repo root")
	rootCmd.PersistentFlags().DurationVar(&timeoutFlag, "timeout", deadline.Default,
		"Maximum time to wait for the pool lock or a git/jj subprocess, overriding TREEHOUSE_TIMEOUT; 0 waits forever. Does not bound the subshell or lifecycle hooks")
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}
