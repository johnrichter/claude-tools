package cli

import (
	"github.com/spf13/cobra"

	"github.com/johnrichter/claude-shared-tooling/go/sysops"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run -- <command> [args...]",
		Short: "Run a subprocess with a bounded timeout and captured, size-capped output",
		Args:  cobra.MinimumNArgs(1),
		Example: `  claude-tools run -- go test ./...
  claude-tools run --timeout 10s --dir /tmp -- ls -la`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd.Flags())
			if err != nil {
				return finishErr(cmd, "internal.config.load_failed", "load configuration", err)
			}
			dir, _ := cmd.Flags().GetString("dir")

			opts := sysops.Options{
				Dir:            dir,
				Timeout:        cfg.Timeout,
				MaxStdoutBytes: cfg.MaxStdoutBytes,
				MaxStderrBytes: cfg.MaxStderrBytes,
			}
			res, runErr := sysops.Run(cmd.Context(), args[0], args[1:], opts)
			if runErr != nil {
				return finishErr(cmd, "internal.sysops.run_failed", "run subprocess", runErr)
			}

			data := map[string]any{
				"exit_code":        res.ExitCode,
				"stdout":           string(res.Stdout),
				"stderr":           string(res.Stderr),
				"stdout_truncated": res.StdoutTruncated,
				"stderr_truncated": res.StderrTruncated,
				"duration_ms":      res.Duration.Milliseconds(),
			}
			result, buildErr := clikitSuccess(cmd, data)
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			// A non-zero child exit is a normal, structured outcome (still
			// clikit-success — the subprocess ran to completion) that a
			// caller inspects via data.exit_code, not a clikit failure class.
			return finish(cmd, result)
		},
	}
	cmd.Flags().String("dir", "", "working directory for the subprocess (default the caller's cwd)")
	return cmd
}
