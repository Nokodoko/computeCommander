// Command hook-bridge is the CLI entry point for the Go-TypeScript bridge.
//
// It reads a BridgeRequest from stdin, dispatches it to the appropriate Go
// hook handler, and writes a BridgeResponse to stdout. It also supports
// listing registered hooks, validating the manifest, and generating
// TypeScript type definitions.
//
// Usage:
//
//	hook-bridge <hook-name>              # dispatch (stdin JSON → stdout JSON)
//	hook-bridge --list                   # list registered hooks
//	hook-bridge --validate               # validate manifest against registry
//	hook-bridge --generate               # generate TypeScript types
//	hook-bridge --manifest <path>        # override manifest location
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/noko/computecommander/bridge"
	"github.com/noko/computecommander/bridge/types"
)

func main() {
	var (
		listFlag     bool
		validateFlag bool
		generateFlag bool
		manifestPath string
	)

	flag.BoolVar(&listFlag, "list", false, "list registered hooks")
	flag.BoolVar(&validateFlag, "validate", false, "validate manifest against registry")
	flag.BoolVar(&generateFlag, "generate", false, "generate TypeScript type definitions")
	flag.StringVar(&manifestPath, "manifest", defaultManifestPath(), "path to manifest.json")
	flag.Parse()

	registry := bridge.NewRegistry()
	registerBuiltinHooks(registry)

	switch {
	case listFlag:
		doList(registry)
	case validateFlag:
		doValidate(registry, manifestPath)
	case generateFlag:
		doGenerate()
	default:
		args := flag.Args()
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "usage: hook-bridge [--list|--validate|--generate] <hook-name>")
			os.Exit(1)
		}
		doDispatch(registry, manifestPath, args[0])
	}
}

// defaultManifestPath returns ~/.claude/bridge/manifest.json.
func defaultManifestPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "manifest.json"
	}
	return filepath.Join(home, ".claude", "bridge", "manifest.json")
}

// doList prints all registered hook names, sorted.
func doList(registry *bridge.Registry) {
	names := registry.Names()
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(n)
	}
}

// doValidate loads the manifest and checks all hooks have handlers.
func doValidate(registry *bridge.Registry, manifestPath string) {
	manifest, err := bridge.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading manifest: %v\n", err)
		os.Exit(1)
	}

	missing := bridge.ValidateManifest(manifest, registry)
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing handlers for: %s\n", strings.Join(missing, ", "))
		os.Exit(1)
	}

	fmt.Printf("manifest OK: %d hooks, all handlers registered\n", len(manifest.Hooks))
}

// doGenerate runs the TypeScript type generator against bridge source files.
func doGenerate() {
	// Find bridge.go relative to the binary or project root.
	inputs := []string{
		findBridgeSource(),
	}

	outputPath := findGeneratedOutput()

	if err := types.GenerateToFile(inputs, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "generate error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("generated: %s\n", outputPath)
}

// doDispatch reads a BridgeRequest from stdin, dispatches, writes BridgeResponse.
func doDispatch(registry *bridge.Registry, manifestPath, hookName string) {
	manifest, err := bridge.LoadManifest(manifestPath)
	if err != nil {
		writeError(fmt.Sprintf("load manifest: %v", err))
		return
	}

	binding, err := bridge.FindBinding(manifest, hookName)
	if err != nil {
		writeError(fmt.Sprintf("find binding: %v", err))
		return
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		writeError(fmt.Sprintf("read stdin: %v", err))
		return
	}

	var req bridge.BridgeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		writeError(fmt.Sprintf("parse request: %v", err))
		return
	}

	// Ensure the hook field matches.
	req.Hook = hookName

	resp, err := bridge.Dispatch(registry, binding, &req)
	if err != nil {
		writeError(err.Error())
		return
	}

	out, err := json.Marshal(resp)
	if err != nil {
		writeError(fmt.Sprintf("marshal response: %v", err))
		return
	}

	os.Stdout.Write(out)
	os.Stdout.Write([]byte("\n"))
}

// writeError writes a failure BridgeResponse to stdout.
func writeError(msg string) {
	resp := bridge.BridgeResponse{
		Success: false,
		Error:   msg,
	}
	data, _ := json.Marshal(resp)
	os.Stdout.Write(data)
	os.Stdout.Write([]byte("\n"))
}

// registerBuiltinHooks registers the default set of Go hook handlers.
// For now this is empty — handlers are added as hooks are implemented.
func registerBuiltinHooks(_ *bridge.Registry) {
	// TODO: Register cmdr-bridge and other Go hook handlers here.
	// Example:
	//   registry.Register("cmdr-bridge", cmdrBridgeHandler)
}

// findBridgeSource locates bridge/bridge.go from common paths.
func findBridgeSource() string {
	candidates := []string{
		"bridge/bridge.go",
		"../bridge/bridge.go",
		filepath.Join(projectRoot(), "bridge", "bridge.go"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "bridge/bridge.go"
}

// findGeneratedOutput returns the path for generated.d.ts.
func findGeneratedOutput() string {
	candidates := []string{
		"bridge/types/generated.d.ts",
		filepath.Join(projectRoot(), "bridge", "types", "generated.d.ts"),
	}
	for _, c := range candidates {
		dir := filepath.Dir(c)
		if _, err := os.Stat(dir); err == nil {
			return c
		}
	}
	return "bridge/types/generated.d.ts"
}

// projectRoot attempts to find the project root via go.mod or git.
func projectRoot() string {
	// Walk up from cwd looking for go.mod.
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
