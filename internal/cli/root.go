// Package cli wires claude-tools' cobra command tree onto the shared
// sysops, webfetch, state and clikit libraries. Every command emits exactly
// one clikit.Result to stdout and exits with that result's exit code —
// cobra's own usage/error printing is silenced so it never competes with
// that one record.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/johnrichter/claude-shared-tooling/go/clikit"
)

// exitError carries a clikit-derived exit code up through cobra's error
// return path without cobra printing anything itself — the command that
// raised it has already emitted its clikit.Result.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

// newRootCmd builds the command tree: platform detection, resource guards,
// bounded process execution, remote sitemap/article/feed fetches, and the
// small crash-safe JSON state store.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "claude-tools",
		Short: "OS/process/hardware, remote-fetch and state operations any session offloads",
		Long: `claude-tools composes the shared sysops, webfetch, state and clikit
libraries into one CLI: detect the host platform, guard against resource
exhaustion before a risky step, run a bounded/capturing subprocess, fetch a
site's sitemap/article/feed metadata, and read/write a small crash-safe JSON
state file.`,
		Example: strings.TrimLeft(`
  claude-tools platform
  claude-tools guard preflight --min-free-memory-bytes 536870912
  claude-tools exec -- go test ./...
  claude-tools fetch article https://example.com/post
  claude-tools state set --path .claude-tools/state.json --key last_run --value '"2026-07-27"'
`, "\n"),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("config", "", "path to a YAML config file (flag > env > file > default)")
	root.PersistentFlags().Duration("timeout", 0, "default timeout for a subprocess or remote fetch (default 30s)")
	root.PersistentFlags().Int("max-stdout-bytes", 0, "captured stdout cap in bytes for exec (default 1MiB)")
	root.PersistentFlags().Int("max-stderr-bytes", 0, "captured stderr cap in bytes for exec (default 1MiB)")

	root.AddCommand(newPlatformCmd())
	root.AddCommand(newGuardCmd())
	root.AddCommand(newExecCmd())
	root.AddCommand(newFetchCmd())
	root.AddCommand(newStateCmd())
	return root
}

// Execute runs the command tree and returns the process exit code —
// clikit's, for anything that reached a subcommand, or a usage code for an
// invocation cobra itself rejected before that (e.g. an unknown flag).
func Execute() int {
	root := newRootCmd()
	ranCmd, err := root.ExecuteC()
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return emitUsageError(ranCmd, err)
}

// commandPath renders cmd's full invocation ("claude-tools fetch sitemap")
// as the token slice clikit.Result.Command requires.
func commandPath(cmd *cobra.Command) []string {
	return strings.Fields(cmd.CommandPath())
}

// sanitizeMessage collapses msg to the single control-character-free line
// clikit.NewError requires: an underlying OS/network error's text is
// free-form and may contain newlines or other control characters a
// diagnostic message may not.
func sanitizeMessage(msg string) string {
	folded := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, msg)
	joined := strings.Join(strings.Fields(folded), " ")
	const max = 4096
	if len(joined) > max {
		joined = joined[:max]
	}
	return joined
}

// emitUsageError handles an error cobra raised before any subcommand's RunE
// ran (bad flag, unknown subcommand) — no clikit.Result has been emitted
// yet, so this is the one place that builds one for that case.
func emitUsageError(cmd *cobra.Command, err error) int {
	diag, buildErr := clikit.NewError(
		"usage.cli.invalid_invocation",
		sanitizeMessage(err.Error()),
		clikit.Manual("run `claude-tools --help` (or `claude-tools <command> --help`) for valid flags and usage"),
		nil,
	)
	if buildErr != nil {
		fmt.Fprintln(os.Stderr, err)
		return clikit.StatusInternal.ExitCode()
	}
	result, buildErr := clikit.NewUsage(commandPath(cmd), nil, []clikit.Diagnostic{diag}, nil)
	if buildErr != nil {
		fmt.Fprintln(os.Stderr, err)
		return clikit.StatusInternal.ExitCode()
	}
	if emitErr := clikit.Emit(os.Stdout, result); emitErr != nil {
		fmt.Fprintln(os.Stderr, emitErr)
	}
	return result.ExitCode
}

// finish emits result and turns it into cobra's error-return path: nil for
// success, an *exitError carrying result.ExitCode otherwise.
func finish(cmd *cobra.Command, result *clikit.Result) error {
	if err := clikit.Emit(cmd.OutOrStdout(), result); err != nil {
		return err
	}
	if result.ExitCode == 0 {
		return nil
	}
	return &exitError{code: result.ExitCode}
}

// finishErr builds and emits a clikit.StatusInternal result for err — an
// infrastructure failure from this CLI itself, not a diagnostic the
// underlying operation reported. code must be in the "internal" class.
func finishErr(cmd *cobra.Command, code, message string, err error) error {
	diag, buildErr := clikit.NewError(code, sanitizeMessage(fmt.Sprintf("%s: %s", message, err)), clikit.Manual("retry; if this persists, file an issue with the log output"), nil)
	if buildErr != nil {
		return buildErr
	}
	result, buildErr := clikit.NewInternal(commandPath(cmd), nil, []clikit.Diagnostic{diag}, nil)
	if buildErr != nil {
		return buildErr
	}
	return finish(cmd, result)
}

// finishUsage builds and emits a clikit.StatusUsage result: the invocation
// itself is wrong (a required flag missing, an unparseable value) and
// nothing was attempted. code must be in the "usage" class.
func finishUsage(cmd *cobra.Command, code, message string) error {
	diag, buildErr := clikit.NewError(
		code, sanitizeMessage(message),
		clikit.Manual(fmt.Sprintf("run `%s --help` for valid flags and usage", cmd.CommandPath())),
		nil,
	)
	if buildErr != nil {
		return buildErr
	}
	result, buildErr := clikit.NewUsage(commandPath(cmd), nil, []clikit.Diagnostic{diag}, nil)
	if buildErr != nil {
		return buildErr
	}
	return finish(cmd, result)
}

// clikitSuccess builds a clikit.StatusSuccess result for cmd carrying data.
func clikitSuccess(cmd *cobra.Command, data map[string]any) (*clikit.Result, error) {
	return clikit.NewSuccess(commandPath(cmd), data)
}

// finishPreconditionUnmet builds and emits a clikit.StatusPreconditionUnmet
// result for err: a required condition (a resource floor, an unmet gate)
// was checked and found wanting, as distinct from an infrastructure failure
// in the check itself.
func finishPreconditionUnmet(cmd *cobra.Command, code string, err error) error {
	diag, buildErr := clikit.NewError(code, sanitizeMessage(err.Error()), clikit.Manual("raise the host resource, lower the required floor, or retry once the condition clears"), nil)
	if buildErr != nil {
		return finishErr(cmd, "internal.result.build_failed", "build diagnostic", buildErr)
	}
	result, buildErr := clikit.NewPreconditionUnmet(commandPath(cmd), nil, []clikit.Diagnostic{diag}, nil)
	if buildErr != nil {
		return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
	}
	return finish(cmd, result)
}
