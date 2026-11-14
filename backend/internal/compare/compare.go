// Package compare diffs two evaluations at evaluation, scenario and turn level.
// Results are matched by scenario key and turn index, so evaluations of
// different suite versions can be compared; a changed definition is flagged so
// a verdict change caused by a rubric edit is not mistaken for an agent change.
package compare

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/jsolana/hiring-lab/part-2/backend/internal/domain"
	"github.com/jsolana/hiring-lab/part-2/backend/internal/report"
)

type Change string

const (
	Unchanged Change = "unchanged"
	Changed   Change = "changed" // definition (question or criteria) differs
	Added     Change = "added"
	Removed   Change = "removed"
)

type Outcome string

const (
	Regression    Outcome = "regression"
	Improvement   Outcome = "improvement"
	StablePass    Outcome = "stable_pass"
	StableFail    Outcome = "stable_fail"
	NotComparable Outcome = "not_comparable" // missing on one side, not executed or execution error
)

type ScenarioSide struct {
	Status          domain.ScenarioStatus `json:"status"`
	FirstFailedTurn int                   `json:"firstFailedTurn"`
	TurnVerdicts    []string              `json:"turnVerdicts"` // pass | fail | error | not_run
	Tokens          int                   `json:"tokens"`
	DurationMs      int64                 `json:"durationMs"`
	Tools           map[string]int        `json:"tools"`
}

type ScenarioDiff struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Change      Change         `json:"change"`
	Outcome     Outcome        `json:"outcome"`
	Base        *ScenarioSide  `json:"base"`
	Target      *ScenarioSide  `json:"target"`
	DeltaTokens int            `json:"deltaTokens"`
	DeltaMs     int64          `json:"deltaMs"`
	ToolDelta   map[string]int `json:"toolDelta"`
}

type ConfigChange struct {
	Path   string `json:"path"`
	Base   any    `json:"base"`
	Target any    `json:"target"`
}

type EvaluationComparison struct {
	BaseID                int64           `json:"baseId"`
	TargetID              int64           `json:"targetId"`
	Base                  report.Summary  `json:"base"`
	Target                report.Summary  `json:"target"`
	DeltaScenarioPassRate float64         `json:"deltaScenarioPassRate"`
	DeltaTurnPassRate     float64         `json:"deltaTurnPassRate"`
	DeltaTokens           int             `json:"deltaTokens"`
	DeltaMs               int64           `json:"deltaMs"`
	Outcomes              map[Outcome]int `json:"outcomes"`
	Changes               map[Change]int  `json:"changes"`
	ConfigDiff            []ConfigChange  `json:"configDiff"`
	Scenarios             []ScenarioDiff  `json:"scenarios"`
}

type TurnDiff struct {
	Index       int                `json:"index"`
	Change      Change             `json:"change"`
	Outcome     Outcome            `json:"outcome"`
	Base        *report.TurnReport `json:"base"`
	Target      *report.TurnReport `json:"target"`
	DeltaTokens int                `json:"deltaTokens"`
	DeltaMs     int64              `json:"deltaMs"`
	ToolDelta   map[string]int     `json:"toolDelta"`
}

type ScenarioComparison struct {
	Key     string        `json:"key"`
	Change  Change        `json:"change"`
	Outcome Outcome       `json:"outcome"`
	Turns   []TurnDiff    `json:"turns"`
	Base    *ScenarioSide `json:"base"`
	Target  *ScenarioSide `json:"target"`
}

// Evaluations compares target against base. Scenarios follow target order;
// scenarios only in base are appended as removed.
func Evaluations(base, target report.EvaluationReport) EvaluationComparison {
	c := EvaluationComparison{
		BaseID: base.Evaluation.ID, TargetID: target.Evaluation.ID, Base: base.Summary, Target: target.Summary,
		DeltaScenarioPassRate: target.Summary.ScenarioPassRate - base.Summary.ScenarioPassRate,
		DeltaTurnPassRate:     target.Summary.TurnPassRate - base.Summary.TurnPassRate,
		DeltaTokens:           target.Summary.Tokens - base.Summary.Tokens,
		DeltaMs:               target.Summary.DurationMs - base.Summary.DurationMs,
		Outcomes:              map[Outcome]int{}, Changes: map[Change]int{},
		ConfigDiff: ConfigDiff(base.Evaluation.EffectiveConfig, target.Evaluation.EffectiveConfig),
		Scenarios:  []ScenarioDiff{},
	}
	add := func(b, t *report.ScenarioReport) {
		sc := Scenarios(b, t)
		d := ScenarioDiff{Key: sc.Key, Change: sc.Change, Outcome: sc.Outcome, Base: sc.Base, Target: sc.Target, ToolDelta: toolDelta(sideTools(sc.Base), sideTools(sc.Target))}
		if t != nil {
			d.Name = t.Name
		} else {
			d.Name = b.Name
		}
		if b != nil && t != nil {
			d.DeltaTokens, d.DeltaMs = t.TotalTokens-b.TotalTokens, t.TotalMs-b.TotalMs
		}
		c.Outcomes[d.Outcome]++
		c.Changes[d.Change]++
		c.Scenarios = append(c.Scenarios, d)
	}
	for i := range target.Scenarios {
		t := &target.Scenarios[i]
		add(base.Scenario(t.Key), t)
	}
	for i := range base.Scenarios {
		if target.Scenario(base.Scenarios[i].Key) == nil {
			add(&base.Scenarios[i], nil)
		}
	}
	return c
}

// Scenarios compares one scenario (either side may be nil) turn by turn.
func Scenarios(base, target *report.ScenarioReport) ScenarioComparison {
	c := ScenarioComparison{Base: side(base), Target: side(target)}
	switch {
	case base == nil:
		c.Key, c.Change = target.Key, Added
	case target == nil:
		c.Key, c.Change = base.Key, Removed
	default:
		c.Key, c.Change = target.Key, Unchanged
	}
	n := 0
	if base != nil {
		n = len(base.Turns)
	}
	if target != nil && len(target.Turns) > n {
		n = len(target.Turns)
	}
	for i := 1; i <= n; i++ {
		td := Turns(base.Turn(i), target.Turn(i))
		td.Index = i
		if c.Change == Unchanged && td.Change != Unchanged {
			c.Change = Changed
		}
		c.Turns = append(c.Turns, td)
	}
	c.Outcome = NotComparable
	if base != nil && target != nil {
		c.Outcome = outcome(scenarioVerdict(base.Status), scenarioVerdict(target.Status))
	}
	return c
}

// Turns compares one turn (either side may be nil).
func Turns(base, target *report.TurnReport) TurnDiff {
	d := TurnDiff{Base: base, Target: target, Outcome: NotComparable}
	switch {
	case base == nil && target == nil:
		return d
	case base == nil:
		d.Index, d.Change = target.Index, Added
	case target == nil:
		d.Index, d.Change = base.Index, Removed
	default:
		d.Index, d.Change = target.Index, Unchanged
		if base.ContentHash != target.ContentHash {
			d.Change = Changed
		}
		d.Outcome = outcome(turnVerdict(base), turnVerdict(target))
		d.DeltaTokens, d.DeltaMs = target.Tokens-base.Tokens, target.DurationMs-base.DurationMs
	}
	d.ToolDelta = toolDelta(turnTools(base), turnTools(target))
	return d
}

func outcome(base, target string) Outcome {
	switch {
	case base == "pass" && target == "fail":
		return Regression
	case base == "fail" && target == "pass":
		return Improvement
	case base == "pass" && target == "pass":
		return StablePass
	case base == "fail" && target == "fail":
		return StableFail
	default:
		return NotComparable
	}
}

func scenarioVerdict(s domain.ScenarioStatus) string {
	switch s {
	case domain.ScenarioPassed:
		return "pass"
	case domain.ScenarioFailed:
		return "fail"
	case domain.ScenarioExecutionError:
		return "error"
	default:
		return "not_run"
	}
}

func turnVerdict(t *report.TurnReport) string {
	switch {
	case t == nil || t.Status == report.StatusNotRun || t.Status == domain.TurnPending || t.Status == domain.TurnRunning:
		return "not_run"
	case t.Status == domain.TurnExecutionError:
		return "error"
	case t.Passed:
		return "pass"
	default:
		return "fail"
	}
}

func side(s *report.ScenarioReport) *ScenarioSide {
	if s == nil {
		return nil
	}
	out := &ScenarioSide{Status: s.Status, FirstFailedTurn: s.FirstFailedTurn, Tokens: s.TotalTokens, DurationMs: s.TotalMs, Tools: s.Tools.Counts, TurnVerdicts: []string{}}
	for i := range s.Turns {
		out.TurnVerdicts = append(out.TurnVerdicts, turnVerdict(&s.Turns[i]))
	}
	return out
}

func sideTools(s *ScenarioSide) map[string]int {
	if s == nil {
		return nil
	}
	return s.Tools
}

func turnTools(t *report.TurnReport) map[string]int {
	if t == nil {
		return nil
	}
	return t.Tools.Counts
}

func toolDelta(base, target map[string]int) map[string]int {
	d := map[string]int{}
	for k, v := range target {
		d[k] += v
	}
	for k, v := range base {
		d[k] -= v
	}
	return d
}

// ConfigDiff lists the leaf values that differ between two JSON documents.
func ConfigDiff(base, target json.RawMessage) []ConfigChange {
	b, t := flatten(base), flatten(target)
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range t {
		keys[k] = true
	}
	var out []ConfigChange
	for k := range keys {
		if !reflect.DeepEqual(b[k], t[k]) {
			out = append(out, ConfigChange{Path: k, Base: b[k], Target: t[k]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func flatten(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return out
	}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		m, ok := v.(map[string]any)
		if !ok {
			out[prefix] = v
			return
		}
		for k, child := range m {
			p := k
			if prefix != "" {
				p = fmt.Sprintf("%s.%s", prefix, k)
			}
			walk(p, child)
		}
	}
	walk("", v)
	return out
}
