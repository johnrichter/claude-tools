package cli

import "testing"

// TestRootCmdSurfacesEverySubcommand is the CLI-shape sanity check: platform, guard,
// exec, fetch and state (sysops + webfetch + state, per SC-STACK) are all present, and
// --help exits clean without SilenceUsage/SilenceErrors surfacing a cobra-printed
// usage line ahead of clikit's own Result.
func TestRootCmdSurfacesEverySubcommand(t *testing.T) {
	root := newRootCmd()

	want := []string{"platform", "guard", "exec", "fetch", "state"}
	for _, name := range want {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Errorf("root command has no %q subcommand: %v", name, err)
		}
	}

	root.SetArgs([]string{"--help"})
	root.SetOut(discard{})
	root.SetErr(discard{})
	if err := root.Execute(); err != nil {
		t.Errorf("--help returned an error: %v", err)
	}
}

// discard is an io.Writer that drops everything written to it.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
