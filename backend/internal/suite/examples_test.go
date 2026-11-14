package suite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/criteria"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/suite"
)

// The suites shipped in part-2/scenarios must stay valid with the server's
// default minimum of 3 turns.
func TestShippedSuitesAreValid(t *testing.T) {
	files, _ := filepath.Glob("../../../scenarios/*.suite.yaml")
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
