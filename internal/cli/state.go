package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/johnrichter/claude-shared-tooling/go/state"
)

// genericSchemaVersion is the schema version a claude-tools-managed ad hoc
// state file is read/written at. get/set treat the document as a plain,
// unversioned bag of keys — a consumer wanting state's richer task/registry
// semantics uses RegisterSource/SeenSource (below) or state's own library
// directly, not this generic surface.
const genericSchemaVersion = 1

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Read and write a small crash-safe JSON state file",
	}
	cmd.AddCommand(newStateGetCmd())
	cmd.AddCommand(newStateSetCmd())
	cmd.AddCommand(newStateRegisterSourceCmd())
	cmd.AddCommand(newStateSeenSourceCmd())
	return cmd
}

func newStateGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Read one key from a state file",
		Args:    cobra.NoArgs,
		Example: `  claude-tools state get --path .claude-tools/state.json --key last_run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			key, _ := cmd.Flags().GetString("key")
			if path == "" || key == "" {
				return finishUsage(cmd, "usage.state.missing_flag", "--path and --key are required")
			}

			doc, err := state.Read(path, genericSchemaVersion, state.Migrations{})
			if err != nil {
				return finishErr(cmd, "internal.state.read_failed", "read state file", err)
			}
			value, found := doc[key]
			result, buildErr := clikitSuccess(cmd, map[string]any{"path": path, "key": key, "value": value, "found": found})
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
	cmd.Flags().String("path", "", "state file path (required)")
	cmd.Flags().String("key", "", "top-level key to read (required)")
	return cmd
}

func newStateSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set",
		Short:   "Write one key in a state file, creating it if absent",
		Args:    cobra.NoArgs,
		Example: `  claude-tools state set --path .claude-tools/state.json --key last_run --value '"2026-07-27"'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			key, _ := cmd.Flags().GetString("key")
			rawValue, _ := cmd.Flags().GetString("value")
			if path == "" || key == "" {
				return finishUsage(cmd, "usage.state.missing_flag", "--path and --key are required")
			}

			var value any
			if err := json.Unmarshal([]byte(rawValue), &value); err != nil {
				return finishUsage(cmd, "usage.state.invalid_value", "--value must be valid JSON (quote a bare string, e.g. '\"text\"')")
			}

			lockErr := state.WithLock(path, func() error {
				doc, readErr := state.Read(path, genericSchemaVersion, state.Migrations{})
				if readErr != nil {
					return readErr
				}
				doc[key] = value
				return state.Write(path, doc, 0o644)
			})
			if lockErr != nil {
				return finishErr(cmd, "internal.state.write_failed", "write state file", lockErr)
			}

			result, buildErr := clikitSuccess(cmd, map[string]any{"path": path, "key": key, "value": value})
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
	cmd.Flags().String("path", "", "state file path (required)")
	cmd.Flags().String("key", "", "top-level key to write (required)")
	cmd.Flags().String("value", "", "JSON-encoded value to store (required)")
	return cmd
}

func newStateRegisterSourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "register-source",
		Short:   "Record a source ref as seen by a consumer, for cross-run dedup",
		Args:    cobra.NoArgs,
		Example: `  claude-tools state register-source --path .claude-tools/registry.json --ref https://example.com/post --consumer psa-signal-discovery`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			ref, _ := cmd.Flags().GetString("ref")
			consumer, _ := cmd.Flags().GetString("consumer")
			if path == "" || ref == "" || consumer == "" {
				return finishUsage(cmd, "usage.state.missing_flag", "--path, --ref and --consumer are required")
			}

			at := state.Now()
			if err := state.RegisterSource(path, ref, consumer, at); err != nil {
				return finishErr(cmd, "internal.state.register_source_failed", "register source", err)
			}
			result, buildErr := clikitSuccess(cmd, map[string]any{"path": path, "ref": ref, "consumer": consumer, "at": at})
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
	cmd.Flags().String("path", "", "source registry file path (required)")
	cmd.Flags().String("ref", "", "source ref to register (required)")
	cmd.Flags().String("consumer", "", "name of the consumer registering ref (required)")
	return cmd
}

func newStateSeenSourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "seen-source",
		Short:   "Report whether a source ref has been registered by any consumer",
		Args:    cobra.NoArgs,
		Example: `  claude-tools state seen-source --path .claude-tools/registry.json --ref https://example.com/post`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			ref, _ := cmd.Flags().GetString("ref")
			if path == "" || ref == "" {
				return finishUsage(cmd, "usage.state.missing_flag", "--path and --ref are required")
			}

			seen := state.SeenSource(path, ref)
			result, buildErr := clikitSuccess(cmd, map[string]any{"path": path, "ref": ref, "seen": seen})
			if buildErr != nil {
				return finishErr(cmd, "internal.result.build_failed", "build result", buildErr)
			}
			return finish(cmd, result)
		},
	}
	cmd.Flags().String("path", "", "source registry file path (required)")
	cmd.Flags().String("ref", "", "source ref to check (required)")
	return cmd
}
