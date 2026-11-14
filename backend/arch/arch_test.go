// Package arch guards the dependency rules that keep the evaluator pluggable.
package arch

import (
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/jsolana/agent-evaluator/backend"

// imports returns the non-test direct imports of every package matching pattern.
func imports(t *testing.T, pattern string) map[string][]string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, pattern).Output()
	if err != nil {
		t.Fatalf("go list %s: %v", pattern, err)
	}
	pkgs := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 {
			pkgs[f[0]] = f[1:]
		}
	}
	if len(pkgs) == 0 {
		t.Fatalf("no packages match %s", pattern)
	}
	return pkgs
}

func forbid(t *testing.T, pattern, prefix, why string) {
	t.Helper()
	for pkg, deps := range imports(t, pattern) {
		for _, d := range deps {
			if strings.HasPrefix(d, prefix) {
				t.Errorf("%s imports %s: %s", pkg, d, why)
			}
		}
	}
}

// agentkit is the public contract: it must stay usable without the rest of the
// module and without third-party code.
func TestAgentkitDependsOnlyOnTheStandardLibrary(t *testing.T) {
	for pkg, deps := range imports(t, module+"/agentkit/...") {
		for _, d := range deps {
			if strings.Contains(strings.SplitN(d, "/", 2)[0], ".") && !strings.HasPrefix(d, module+"/agentkit") {
				t.Errorf("%s imports %s: agentkit may only use the standard library", pkg, d)
			}
		}
	}
}

// Adapters are written by third parties, so the bundled ones must be buildable
// from the public contract alone.
func TestAdaptersUseOnlyThePublicContract(t *testing.T) {
	for _, p := range []string{"/adapters/...", "/examples/..."} {
		forbid(t, module+p, module+"/internal", "adapters must not depend on the core")
		forbid(t, module+p, module+"/evaluator", "adapters must not depend on the composition root")
	}
}

// The core knows the contract, never a concrete adapter.
func TestCoreDoesNotKnowAdapters(t *testing.T) {
	forbid(t, module+"/internal/...", module+"/adapters", "the core must not import concrete adapters")
	forbid(t, module+"/internal/...", module+"/examples", "the core must not import examples")
	forbid(t, module+"/evaluator", module+"/adapters", "adapters are selected by the program's main, not by evaluator")
}
