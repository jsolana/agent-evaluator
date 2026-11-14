package httpagent

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/jsolana/agent-evaluator/backend/agentkit"
)

// Config describes how to talk to a REST agent. See the README for the format.
type Config struct {
	BaseURL      string            `yaml:"baseUrl"`
	Headers      map[string]string `yaml:"headers"`
	Conversation struct {
		Mode string `yaml:"mode"` // stateless (default) | stateful
	} `yaml:"conversation"`
	Turn struct {
		Method            string `yaml:"method"`
		Path              string `yaml:"path"`
		Body              string `yaml:"body"`
		IdempotencyHeader string `yaml:"idempotencyHeader"`
	} `yaml:"turn"`
	Response struct {
		Text   string `yaml:"text"`
		Ref    string `yaml:"ref"`
		Tokens string `yaml:"tokens"`
	} `yaml:"response"`
	Errors struct {
		Retryable []string `yaml:"retryable"` // status codes or classes such as 5xx
		Conflict  []string `yaml:"conflict"`
	} `yaml:"errors"`
	Traces *TracesConfig `yaml:"traces"`
}

// TracesConfig maps an array of the response to spans.
type TracesConfig struct {
	From       string `yaml:"from"`
	Name       string `yaml:"name"`
	Arguments  string `yaml:"arguments"`
	DurationMs string `yaml:"durationMs"`
	Error      string `yaml:"error"`
	Documents  *struct {
		From    string `yaml:"from"`
		Title   string `yaml:"title"`
		URL     string `yaml:"url"`
		Content string `yaml:"content"`
	} `yaml:"documents"`
}

// spec is a validated Config with its derived parts, ready to be used.
type spec struct {
	Config
	stateful  bool
	body      *template.Template
	required  []string // settings referenced as ${NAME}, reported by Check
	retryable statusSet
	conflict  statusSet
	env       agentkit.Env
}

var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// parse reads, validates and prepares a configuration. It fails on unknown
// fields and on anything that could never work, so mistakes show at start-up.
func parse(src io.Reader, env agentkit.Env) (*spec, error) {
	var c Config
	dec := yaml.NewDecoder(src)
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("httpagent config: %w", err)
	}
	s := &spec{Config: c, env: env}

	switch c.Conversation.Mode {
	case "", "stateless":
	case "stateful":
		s.stateful = true
	default:
		return nil, fmt.Errorf("httpagent config: conversation.mode must be stateless or stateful, got %q", c.Conversation.Mode)
	}
	if c.BaseURL == "" || c.Turn.Path == "" || c.Response.Text == "" {
		return nil, errors.New("httpagent config: baseUrl, turn.path and response.text are required")
	}
	if s.stateful && c.Response.Ref == "" {
		return nil, errors.New("httpagent config: a stateful agent needs response.ref")
	}
	if c.Turn.Method == "" {
		s.Turn.Method = "POST"
	}

	s.BaseURL = strings.TrimRight(s.expand(c.BaseURL), "/")
	s.Turn.Path = s.expand(c.Turn.Path)
	s.Headers = make(map[string]string, len(c.Headers))
	for k, v := range c.Headers {
		s.Headers[k] = s.expand(v)
	}

	if c.Turn.Body != "" {
		t, err := template.New("body").Funcs(funcs).Option("missingkey=error").Parse(c.Turn.Body)
		if err != nil {
			return nil, fmt.Errorf("httpagent config: turn.body: %w", err)
		}
		s.body = t
	}

	retry := c.Errors.Retryable
	if retry == nil {
		retry = []string{"429", "5xx"}
	}
	var err error
	if s.retryable, err = newStatusSet(retry); err != nil {
		return nil, fmt.Errorf("httpagent config: errors.retryable: %w", err)
	}
	if s.conflict, err = newStatusSet(c.Errors.Conflict); err != nil {
		return nil, fmt.Errorf("httpagent config: errors.conflict: %w", err)
	}
	if t := c.Traces; t != nil && (t.From == "" || t.Name == "") {
		return nil, errors.New("httpagent config: traces needs from and name")
	}

	seen := map[string]bool{}
	for _, text := range append([]string{c.BaseURL, c.Turn.Path}, mapValues(c.Headers)...) {
		for _, m := range varRef.FindAllStringSubmatch(text, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				s.required = append(s.required, m[1])
			}
		}
	}
	return s, nil
}

// expand replaces ${NAME} with the setting of that name.
func (s *spec) expand(text string) string {
	return varRef.ReplaceAllStringFunc(text, func(m string) string { return s.env(m[2 : len(m)-1]) })
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// statusSet matches HTTP status codes written as 429 or as a class such as 5xx.
type statusSet struct {
	codes   map[int]bool
	classes map[int]bool
}

func newStatusSet(items []string) (statusSet, error) {
	set := statusSet{codes: map[int]bool{}, classes: map[int]bool{}}
	for _, it := range items {
		var code int
		if len(it) == 3 && it[1:] == "xx" && it[0] >= '1' && it[0] <= '5' {
			set.classes[int(it[0]-'0')] = true
		} else if _, err := fmt.Sscanf(it, "%d", &code); err == nil && code >= 100 && code <= 599 {
			set.codes[code] = true
		} else {
			return set, fmt.Errorf("%q is neither a status code nor a class like 5xx", it)
		}
	}
	return set, nil
}

func (s statusSet) has(code int) bool { return s.codes[code] || s.classes[code/100] }

// load parses a configuration held in bytes (used by tests and the factory).
func load(data []byte, env agentkit.Env) (*spec, error) { return parse(bytes.NewReader(data), env) }
