package suite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jsolana/agent-evaluator/backend/internal/criteria"
	"github.com/jsolana/agent-evaluator/backend/internal/suite"
)

// The suites shipped in scenarios/ and in adapters/*/examples/ must stay valid with the server's
// default minimum of 3 turns.
func TestShippedSuitesAreValid(t *testing.T) {
	files, _ := filepath.Glob("../../../scenarios/*.suite.yaml")
	adapterFiles, _ := filepath.Glob("../../adapters/*/examples/*.suite.yaml")
	files = append(files, adapterFiles...)
	if len(files) == 0 {
		t.Fatal("no shipped suites found")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s, err := suite.Parse(src, suite.Options{MinTurns: 3, Validator: criteria.NewRegistry(nil)})
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, sc := range s.Scenarios {
			if len(sc.Turns) < 3 {
				t.Fatalf("%s: scenario %s has %d turns", f, sc.Key, len(sc.Turns))
			}
		}
	}
}
