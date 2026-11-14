package main

import (
	"strings"
	"testing"
)

func TestBuildAgentRejectsUnknownAgents(t *testing.T) {
	if _, _, _, err := buildAgent("other"); err == nil || !strings.Contains(err.Error(), "fake, hiringlab") {
		t.Fatalf("an unknown AGENT must fail listing the known ones, got %v", err)
	}
}

func TestBuildAgentBuildsEveryRegisteredAgent(t *testing.T) {
	for name := range agents {
		ag, traces, check, err := buildAgent(name)
		if err != nil || ag == nil || traces == nil || check == nil {
			t.Fatalf("%s: agent=%v traces=%v check=%v err=%v", name, ag, traces, check, err)
		}
	}
}
