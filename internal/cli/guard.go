package cli

import (
	"github.com/spf13/cobra"

	"github.com/johnrichter/claude-shared-tooling/go/sysops"
)

func newGuardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guard",
		Short: "Preflight host resources before a memory- or descriptor-heavy step",
	}
	cmd.AddCommand(newGuardPreflightCmd())
	cmd.AddCommand(newGuardLimitsCmd())
	return cmd
}

func newGuardPreflightCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "preflight",
		Short:   "Fail fast if free memory or the open-file limit is below a required floor",
		Args:    cobra.NoArgs,
		Example: "  claude-tools guard preflight --min-free-memory-bytes 536870912 --min-open-files 1024",
		RunE: func(cmd *cobra.Command, args []string) error {
			minMem, _ := cmd.Flags().GetUint64("min-free-memory-bytes")
			minFiles, _ := cmd.Flags().GetUint64("min-open-files")

			guard := sysops.NewGuard()
			req := sysops.Requirement{MinFreeMemoryBytes: minMem, MinOpenFiles: minFiles}
			if err := guard.Preflight(req); err != nil {
				return finishPreconditionUnmet(cmd, "precondition_unmet.guard.requirement_not_met", err)
			}

			data := map[string]any{"min_free_memory_bytes": minMem, "min_open_files": minFiles}
			result, buildErr := clikitSuccess(cmd, data)
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
	cmd.Flags().Uint64("min-free-memory-bytes", 0, "required free memory floor in bytes (0 skips the check)")
	cmd.Flags().Uint64("min-open-files", 0, "required open-file soft-limit floor (0 skips the check)")
	return cmd
}

func newGuardLimitsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "limits",
		Short:   "Report current free memory and the open-file soft/hard limit",
		Args:    cobra.NoArgs,
		Example: "  claude-tools guard limits",
		RunE: func(cmd *cobra.Command, args []string) error {
			guard := sysops.NewGuard()
			free, err := guard.FreeMemoryBytes()
			if err != nil {
				return finishErr(cmd, "internal.guard.free_memory_failed", "read free memory", err)
			}
			lim, err := guard.OpenFileLimit()
			if err != nil {
				return finishErr(cmd, "internal.guard.open_file_limit_failed", "read open-file limit", err)
			}

			data := map[string]any{
				"free_memory_bytes": free,
				"open_files_soft":   lim.Soft,
				"open_files_hard":   lim.Hard,
			}
			result, buildErr := clikitSuccess(cmd, data)
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
}
