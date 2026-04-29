package icarus

import "github.com/noko/computecommander/pkg/runtimes"

// init self-registers the Icarus adapter into the global runtime registry so
// that callers can resolve it via runtimes.GetRuntime("icarus") after a
// blank import of this package (mirrors the pattern used by every other
// adapter in pkg/runtimes/).
func init() {
	runtimes.RegisterRuntime(New())
}
