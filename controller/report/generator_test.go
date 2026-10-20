package report

import (
	"strings"
	"testing"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
)

func sampleRun() protocol.BenchmarkRun {
	return protocol.BenchmarkRun{
		ID:        "test-run-1",
		Timestamp: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Topology:  protocol.TopoSingle,
		Workloads: []protocol.WorkloadType{protocol.WorkloadCRUD},
		Duration:  60,
		Agents: []protocol.AgentEntry{
			{Language: protocol.LangGo, Mode: protocol.ModeLibrary, Port: 7003, Host: "localhost"},
			{Language: protocol.LangGo, Mode: protocol.ModeVanilla, Port: 7004, Host: "localhost"},
		},
	}
}

func sampleMetrics() []protocol.MetricSnapshot {
	return []protocol.MetricSnapshot{
		{RunID: "test-run-1", Language: "go", Mode: "library", Workload: "crud", OpsPerSec: 1000, LatencyP50Us: 500, LatencyP95Us: 1500, LatencyP99Us: 3000, Errors: 0, Retries: 0},
		{RunID: "test-run-1", Language: "go", Mode: "library", Workload: "crud", OpsPerSec: 1200, LatencyP50Us: 450, LatencyP95Us: 1400, LatencyP99Us: 2800, Errors: 0, Retries: 0},
		{RunID: "test-run-1", Language: "go", Mode: "vanilla", Workload: "crud", OpsPerSec: 1100, LatencyP50Us: 480, LatencyP95Us: 1450, LatencyP99Us: 2900, Errors: 2, Retries: 1},
		{RunID: "test-run-1", Language: "go", Mode: "vanilla", Workload: "crud", OpsPerSec: 1300, LatencyP50Us: 460, LatencyP95Us: 1420, LatencyP99Us: 2850, Errors: 1, Retries: 0},
	}
}

func TestGenerateHTMLProducesValidHTML(t *testing.T) {
	run := sampleRun()
	metrics := sampleMetrics()

	html, err := GenerateHTML(run, metrics, nil)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content := string(html)
	if !strings.Contains(content, "<!DOCTYPE html>") {
		t.Error("output missing DOCTYPE")
	}
	if !strings.Contains(content, "</html>") {
		t.Error("output missing closing </html> tag")
	}
	if !strings.Contains(content, "test-run-1") {
		t.Error("output missing run ID")
	}
	if !strings.Contains(content, "CockroachDB") {
		t.Error("output missing project name")
	}
}

func TestGenerateHTMLWithChaosEvents(t *testing.T) {
	run := sampleRun()
	chaos := []protocol.ChaosEvent{
		{
			Timestamp: time.Date(2026, 6, 1, 12, 0, 30, 0, time.UTC),
			Scenario:  protocol.ChaosNodeKill,
			Action:    "kill",
			Target:    "crdb-2",
		},
	}

	html, err := GenerateHTML(run, sampleMetrics(), chaos)
	if err != nil {
		t.Fatalf("GenerateHTML with chaos failed: %v", err)
	}

	content := string(html)
	if !strings.Contains(content, "node_kill") {
		t.Error("output missing chaos scenario")
	}
}

func TestBuildAgentSummariesComputesAverages(t *testing.T) {
	metrics := []protocol.MetricSnapshot{
		{Language: "go", Mode: "library", OpsPerSec: 1000, LatencyP50Us: 500, LatencyP95Us: 1500, LatencyP99Us: 3000, Errors: 5, Retries: 2},
		{Language: "go", Mode: "library", OpsPerSec: 2000, LatencyP50Us: 600, LatencyP95Us: 1800, LatencyP99Us: 3600, Errors: 3, Retries: 1},
	}

	summaries := buildAgentSummaries(metrics)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	s := summaries[0]
	if s.Language != "go" {
		t.Errorf("expected language go, got %s", s.Language)
	}
	if s.Mode != "library" {
		t.Errorf("expected mode library, got %s", s.Mode)
	}

	expectedAvgOps := 1500.0
	if s.AvgOpsPerSec != expectedAvgOps {
		t.Errorf("expected avg ops/sec %.1f, got %.1f", expectedAvgOps, s.AvgOpsPerSec)
	}

	if s.TotalErrors != 8 {
		t.Errorf("expected total errors 8, got %d", s.TotalErrors)
	}
	if s.TotalRetries != 3 {
		t.Errorf("expected total retries 3, got %d", s.TotalRetries)
	}
}

func TestBuildAgentSummariesMultipleAgents(t *testing.T) {
	metrics := []protocol.MetricSnapshot{
		{Language: "go", Mode: "library", OpsPerSec: 1000},
		{Language: "go", Mode: "vanilla", OpsPerSec: 1100},
		{Language: "java", Mode: "library", OpsPerSec: 900},
	}

	summaries := buildAgentSummaries(metrics)
	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(summaries))
	}
}

func TestBuildKeyInsightsRetryStormData(t *testing.T) {
	metrics := []protocol.MetricSnapshot{
		{Language: "go", Mode: "library", Workload: "retry_storm", OpsPerSec: 500, Errors: 10, Retries: 50},
		{Language: "go", Mode: "vanilla", Workload: "retry_storm", OpsPerSec: 400, Errors: 100, Retries: 30},
	}

	insights := buildKeyInsights(metrics, []string{"retry_storm"})
	if len(insights) == 0 {
		t.Fatal("expected at least one insight from retry storm data")
	}

	found := false
	for _, insight := range insights {
		if strings.Contains(insight.Title, "go") && strings.Contains(insight.Title, "higher") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected insight about vanilla retry failures being higher for go")
	}
}

func TestBuildKeyInsightsNoData(t *testing.T) {
	insights := buildKeyInsights(nil, []string{"crud"})
	if len(insights) == 0 {
		t.Fatal("expected fallback insight when no data")
	}
	if !strings.Contains(insights[0].Title, "Run more workloads") {
		t.Errorf("expected fallback insight text, got %q", insights[0].Title)
	}
}

func TestBuildProsConsWithData(t *testing.T) {
	metrics := []protocol.MetricSnapshot{
		{Language: "go", Mode: "library", Workload: "crud", OpsPerSec: 950},
		{Language: "go", Mode: "vanilla", Workload: "crud", OpsPerSec: 1000},
		{Language: "go", Mode: "library", Workload: "retry_storm", OpsPerSec: 500, Errors: 10, Retries: 50},
		{Language: "go", Mode: "vanilla", Workload: "retry_storm", OpsPerSec: 400, Errors: 100, Retries: 30},
	}

	result := buildProsCons(metrics)
	if len(result) == 0 {
		t.Fatal("expected at least one language in pros/cons")
	}

	var goEntry *LangProsCons
	for i := range result {
		if result[i].Language == "go" {
			goEntry = &result[i]
			break
		}
	}
	if goEntry == nil {
		t.Fatal("expected go language in pros/cons")
	}
	if len(goEntry.Pros) == 0 {
		t.Error("expected at least one pro for go")
	}
}

func TestBuildProsConsNoData(t *testing.T) {
	result := buildProsCons(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 entries with no data, got %d", len(result))
	}
}

func TestFormatLatency(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{500, "500us"},
		{1500, "1.50ms"},
		{1500000, "1.5s"},
	}
	for _, tt := range tests {
		got := formatLatency(tt.input)
		if got != tt.want {
			t.Errorf("formatLatency(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
