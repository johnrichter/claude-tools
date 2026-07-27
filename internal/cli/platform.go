package cli

import (
	"github.com/spf13/cobra"

	"github.com/johnrichter/claude-shared-tooling/go/sysops"
)

func newPlatformCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "platform",
		Short:   "Report the running host's operating system and CPU architecture",
		Args:    cobra.NoArgs,
		Example: "  claude-tools platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := sysops.Platform()
			result, err := clikitSuccess(cmd, map[string]any{"os": p.OS, "arch": p.Arch, "platform": p.String()})
			if err != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", err)
			}
			return finish(cmd, result)
		},
	}
}
