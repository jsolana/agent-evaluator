// Command convert turns the legacy scenarios format (expectedAnswer,
// expectedTools, forbiddenTools, sources per turn) into a suite with explicit
// criteria. Usage: go run ./cmd/hiringlab-convert < old.yaml > suite.yaml
// The minimum number of turns is enforced by the server (SUITE_MIN_TURNS).
package main

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type legacySuite struct {
	Name      string `yaml:"name"`
	Scenarios []struct {
		ID    string   `yaml:"id"`
		Name  string   `yaml:"name"`
		Tags  []string `yaml:"tags"`
		Turns []struct {
			Question       string   `yaml:"question"`
			ExpectedAnswer string   `yaml:"expectedAnswer"`
			ExpectedTools  []string `yaml:"expectedTools"`
			ForbiddenTools []string `yaml:"forbiddenTools"`
			Sources        []string `yaml:"sources"`
		} `yaml:"turns"`
	} `yaml:"scenarios"`
}

type outCriterion map[string]any

type outTurn struct {
	Question string         `yaml:"question"`
	Criteria []outCriterion `yaml:"criteria"`
}

type outScenario struct {
	Key   string    `yaml:"key,omitempty"`
	Name  string    `yaml:"name,omitempty"`
	Tags  []string  `yaml:"tags,omitempty"`
	Turns []outTurn `yaml:"turns"`
}

type outSuite struct {
	Name      string        `yaml:"name"`
	Scenarios []outScenario `yaml:"scenarios"`
}

func convert(src []byte) ([]byte, error) {
	var in legacySuite
	if err := yaml.Unmarshal(src, &in); err != nil {
		return nil, err
	}
	out := outSuite{Name: in.Name}
	for _, s := range in.Scenarios {
		sc := outScenario{Key: s.ID, Name: s.Name, Tags: s.Tags}
		for _, t := range s.Turns {
			ot := outTurn{Question: t.Question}
			if t.ExpectedAnswer != "" {
				ot.Criteria = append(ot.Criteria, outCriterion{"type": "llm_judge", "expected": t.ExpectedAnswer})
			}
			if len(t.ExpectedTools) > 0 {
				ot.Criteria = append(ot.Criteria, outCriterion{"type": "tools_required", "tools": t.ExpectedTools})
			}
			if len(t.ForbiddenTools) > 0 {
				ot.Criteria = append(ot.Criteria, outCriterion{"type": "tools_forbidden", "tools": t.ForbiddenTools})
			}
			if len(t.Sources) > 0 {
				ot.Criteria = append(ot.Criteria, outCriterion{"type": "sources_retrieved", "urls": t.Sources})
			}
			sc.Turns = append(sc.Turns, ot)
		}
		out.Scenarios = append(out.Scenarios, sc)
	}
	return yaml.Marshal(out)
}

func main() {
	src, err := io.ReadAll(os.Stdin)
	if err == nil {
		var out []byte
		if out, err = convert(src); err == nil {
			_, err = os.Stdout.Write(out)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "convert:", err)
		os.Exit(1)
	}
}
