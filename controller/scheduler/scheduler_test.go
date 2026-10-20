package scheduler

import (
	"testing"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
)

func TestGroupAgentsByLanguage(t *testing.T) {
	agents := []protocol.AgentEntry{
		{Language: "go", Mode: "library", Port: 7003, Host: "localhost"},
		{Language: "go", Mode: "vanilla", Port: 7004, Host: "localhost"},
		{Language: "java", Mode: "library", Port: 7001, Host: "localhost"},
		{Language: "python", Mode: "vanilla", Port: 7006, Host: "localhost"},
	}

	groups := groupAgentsByLanguage(agents)

	if len(groups) != 3 {
		t.Fatalf("expected 3 language groups, got %d", len(groups))
	}

	if len(groups["go"]) != 2 {
		t.Errorf("expected 2 go agents, got %d", len(groups["go"]))
	}
	if len(groups["java"]) != 1 {
		t.Errorf("expected 1 java agent, got %d", len(groups["java"]))
	}
	if len(groups["python"]) != 1 {
		t.Errorf("expected 1 python agent, got %d", len(groups["python"]))
	}
}

func TestGroupAgentsByLanguageEmpty(t *testing.T) {
	groups := groupAgentsByLanguage(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil agents, got %d", len(groups))
	}
}

func TestGroupAgentsByLanguageSingleLanguage(t *testing.T) {
	agents := []protocol.AgentEntry{
		{Language: "go", Mode: "library", Port: 7003, Host: "localhost"},
		{Language: "go", Mode: "vanilla", Port: 7004, Host: "localhost"},
	}

	groups := groupAgentsByLanguage(agents)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups["go"]) != 2 {
		t.Errorf("expected 2 go agents, got %d", len(groups["go"]))
	}
}

func TestLanguageFiltering(t *testing.T) {
	allAgents := protocol.DefaultAgents
	wantLanguages := map[protocol.Language]bool{
		protocol.LangGo:     true,
		protocol.LangPython: true,
	}

	var filtered []protocol.AgentEntry
	for _, a := range allAgents {
		if wantLanguages[a.Language] {
			filtered = append(filtered, a)
		}
	}

	if len(filtered) != 4 {
		t.Fatalf("expected 4 filtered agents (go lib+van, python lib+van), got %d", len(filtered))
	}

	for _, a := range filtered {
		if !wantLanguages[a.Language] {
			t.Errorf("unexpected language in filtered list: %s", a.Language)
		}
	}
}

func TestNewScheduler(t *testing.T) {
	agents := []protocol.AgentEntry{
		{Language: "go", Mode: "library", Port: 7003, Host: "localhost"},
	}
	s := New(agents, nil)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.IsRunning() {
		t.Error("new scheduler should not be running")
	}
	if s.CurrentRun() != nil {
		t.Error("new scheduler should have nil current run")
	}
}
