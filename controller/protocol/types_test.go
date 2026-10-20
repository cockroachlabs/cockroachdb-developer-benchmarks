package protocol

import (
	"testing"
)

func TestDefaultAgentsPortAssignments(t *testing.T) {
	expected := map[Language]map[RunMode]int{
		LangJava:       {ModeLibrary: 7001, ModeVanilla: 7002},
		LangJavaNative: {ModeLibrary: 7009, ModeVanilla: 7010},
		LangGo:         {ModeLibrary: 7003, ModeVanilla: 7004},
		LangPython:     {ModeLibrary: 7005, ModeVanilla: 7006},
		LangDotNet:     {ModeLibrary: 7007, ModeVanilla: 7008},
	}

	for _, agent := range DefaultAgents {
		modes, ok := expected[agent.Language]
		if !ok {
			t.Errorf("unexpected language in DefaultAgents: %s", agent.Language)
			continue
		}
		wantPort, ok := modes[agent.Mode]
		if !ok {
			t.Errorf("unexpected mode %s for language %s", agent.Mode, agent.Language)
			continue
		}
		if agent.Port != wantPort {
			t.Errorf("%s/%s: got port %d, want %d", agent.Language, agent.Mode, agent.Port, wantPort)
		}
		if agent.Host != "localhost" {
			t.Errorf("%s/%s: got host %q, want %q", agent.Language, agent.Mode, agent.Host, "localhost")
		}
	}
}

func TestDefaultAgentsNoDuplicatePorts(t *testing.T) {
	seen := map[int]string{}
	for _, agent := range DefaultAgents {
		key := string(agent.Language) + "/" + string(agent.Mode)
		if prev, exists := seen[agent.Port]; exists {
			t.Errorf("port %d used by both %s and %s", agent.Port, prev, key)
		}
		seen[agent.Port] = key
	}
}

func TestLanguageConstantsAreUnique(t *testing.T) {
	langs := []Language{LangJava, LangGo, LangPython, LangDotNet, LangJavaNative}
	seen := map[Language]bool{}
	for _, l := range langs {
		if seen[l] {
			t.Errorf("duplicate language constant: %s", l)
		}
		seen[l] = true
	}
	if len(seen) != len(langs) {
		t.Errorf("expected %d unique languages, got %d", len(langs), len(seen))
	}
}

func TestWorkloadTypeConstants(t *testing.T) {
	workloads := []WorkloadType{
		WorkloadCRUD, WorkloadRetryStorm, WorkloadMixedRW,
		WorkloadFollowerReads, WorkloadCDC, WorkloadVectorSearch,
	}
	seen := map[WorkloadType]bool{}
	for _, w := range workloads {
		if seen[w] {
			t.Errorf("duplicate workload constant: %s", w)
		}
		seen[w] = true
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 unique workloads, got %d", len(seen))
	}
}

func TestRunModeConstants(t *testing.T) {
	if ModeLibrary == ModeVanilla {
		t.Error("ModeLibrary and ModeVanilla must be distinct")
	}
	if ModeLibrary != "library" {
		t.Errorf("ModeLibrary = %q, want %q", ModeLibrary, "library")
	}
	if ModeVanilla != "vanilla" {
		t.Errorf("ModeVanilla = %q, want %q", ModeVanilla, "vanilla")
	}
}
