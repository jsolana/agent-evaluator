package criteria

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jsolana/agent-evaluator/backend/internal/domain"
)

const deterministicVersion = "1"

// ---- tools_required / tools_forbidden

type toolsCriterion struct {
	typ      string
	tools    []string
	required bool
}

func toolsFactory(typ string, required bool) Factory {
	return func(params json.RawMessage) (Criterion, error) {
		var p struct {
			Tools []string `json:"tools"`
		}
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		if len(p.Tools) == 0 {
			return nil, errors.New("tools: at least one tool is required")
		}
		return toolsCriterion{typ, p.Tools, required}, nil
	}
}

func (c toolsCriterion) Evaluate(_ context.Context, in Input) (domain.CriterionResult, error) {
	called := map[string]bool{}
	var sequence []string
	for _, s := range in.Spans {
		if s.Kind == "tool" {
			called[s.Name] = true
			sequence = append(sequence, s.Name)
		}
	}
	var offending []string
	for _, t := range c.tools {
		if called[t] != c.required {
			offending = append(offending, t)
		}
	}
	calls := "called: [" + strings.Join(sequence, " → ") + "]"
	if len(offending) == 0 {
		return result(c.typ, deterministicVersion, true, calls), nil
	}
	verb := "not called"
	if !c.required {
		verb = "called but forbidden"
	}
	return result(c.typ, deterministicVersion, false, fmt.Sprintf("%s: %s; %s", verb, strings.Join(offending, ", "), calls)), nil
}

// ---- sources_retrieved

// match "all" (default) requires every url; "any" accepts one of them, useful
// when the corpus holds duplicate pages for the same content.
type sourcesCriterion struct {
	urls []string
	any  bool
}

func sourcesFactory(params json.RawMessage) (Criterion, error) {
	var p struct {
		URLs  []string `json:"urls"`
		Match string   `json:"match"`
	}
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if len(p.URLs) == 0 {
		return nil, errors.New("urls: at least one url is required")
	}
	if p.Match != "" && p.Match != "all" && p.Match != "any" {
		return nil, errors.New(`match must be "all" or "any"`)
	}
	return sourcesCriterion{p.URLs, p.Match == "any"}, nil
}

func (c sourcesCriterion) Evaluate(_ context.Context, in Input) (domain.CriterionResult, error) {
	retrieved := map[string]bool{}
	for _, s := range in.Spans {
		for _, d := range s.Documents {
			retrieved[normalizeURL(d.URL)] = true
		}
	}
	var missing, found []string
	for _, u := range c.urls {
		if retrieved[normalizeURL(u)] {
			found = append(found, u)
		} else {
			missing = append(missing, u)
		}
	}
	if c.any {
		if len(found) == 0 {
			return result("sources_retrieved", deterministicVersion, false, "none of the expected sources retrieved: "+strings.Join(missing, ", ")), nil
		}
		return result("sources_retrieved", deterministicVersion, true, fmt.Sprintf("%d of %d expected sources retrieved", len(found), len(c.urls)), found...), nil
	}
	if len(missing) > 0 {
		return result("sources_retrieved", deterministicVersion, false, "not retrieved: "+strings.Join(missing, ", "), found...), nil
	}
	return result("sources_retrieved", deterministicVersion, true, fmt.Sprintf("all %d expected sources retrieved", len(found)), found...), nil
}

// normalizeURL ignores scheme, case of host, query (tracking params), fragment and trailing slash.
func normalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	return strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
}

// ---- regex

type regexCriterion struct {
	re     *regexp.Regexp
	negate bool
}

func regexFactory(params json.RawMessage) (Criterion, error) {
	var p struct {
		Pattern string `json:"pattern"`
		Negate  bool   `json:"negate"`
	}
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if p.Pattern == "" {
		return nil, errors.New("pattern is required")
	}
	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return nil, fmt.Errorf("pattern: %w", err)
	}
	return regexCriterion{re, p.Negate}, nil
}

func (c regexCriterion) Evaluate(_ context.Context, in Input) (domain.CriterionResult, error) {
	match := c.re.FindString(in.Answer)
	found := c.re.MatchString(in.Answer)
	var evidence []string
	if found {
		evidence = []string{match}
	}
	switch {
	case found && !c.negate:
		return result("regex", deterministicVersion, true, "pattern found: "+c.re.String(), evidence...), nil
	case found && c.negate:
		return result("regex", deterministicVersion, false, "forbidden pattern found: "+c.re.String(), evidence...), nil
	case !c.negate:
		return result("regex", deterministicVersion, false, "pattern not found: "+c.re.String()), nil
	default:
		return result("regex", deterministicVersion, true, "forbidden pattern absent: "+c.re.String()), nil
	}
}
