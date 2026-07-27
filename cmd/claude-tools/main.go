// Command claude-tools composes the shared sysops, webfetch, state and
// clikit libraries into one CLI: OS/process/hardware operations, remote
// sitemap/article/feed fetches, and a small crash-safe JSON state store.
package main

import (
	"os"

	"github.com/johnrichter/claude-tools/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
