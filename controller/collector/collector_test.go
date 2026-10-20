package collector

import (
	"testing"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
)

func TestIngestAndRecentMetrics(t *testing.T) {
	c := New(nil)

	snap := protocol.MetricSnapshot{
		RunID:    "run-1",
		Language: "go",
		Mode:     "library",
		Workload: "crud",
		OpsPerSec: 1000,
	}
	c.Ingest(snap)

	metrics := c.RecentMetrics("", 0)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].RunID != "run-1" {
		t.Errorf("expected run_id %q, got %q", "run-1", metrics[0].RunID)
	}
	if metrics[0].OpsPerSec != 1000 {
		t.Errorf("expected ops_per_sec 1000, got %f", metrics[0].OpsPerSec)
	}
}

func TestRecentMetricsFilterByRunID(t *testing.T) {
	c := New(nil)

	c.Ingest(protocol.MetricSnapshot{RunID: "run-1", Language: "go", Mode: "library", Workload: "crud"})
	c.Ingest(protocol.MetricSnapshot{RunID: "run-2", Language: "go", Mode: "library", Workload: "crud"})
	c.Ingest(protocol.MetricSnapshot{RunID: "run-1", Language: "java", Mode: "vanilla", Workload: "crud"})

	metrics := c.RecentMetrics("run-1", 0)
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics for run-1, got %d", len(metrics))
	}
	for _, m := range metrics {
		if m.RunID != "run-1" {
			t.Errorf("expected run_id run-1, got %s", m.RunID)
		}
	}
}

func TestRecentMetricsWithLimit(t *testing.T) {
	c := New(nil)

	for i := 0; i < 10; i++ {
		c.Ingest(protocol.MetricSnapshot{RunID: "run-1", OpsPerSec: float64(i)})
	}

	metrics := c.RecentMetrics("", 3)
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics with limit, got %d", len(metrics))
	}
	if metrics[0].OpsPerSec != 7 {
		t.Errorf("expected first metric ops_per_sec=7 (tail), got %f", metrics[0].OpsPerSec)
	}
}

func TestSubscribeUnsubscribeNoPanic(t *testing.T) {
	c := New(nil)

	if c.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers, got %d", c.SubscriberCount())
	}

	c.Ingest(protocol.MetricSnapshot{RunID: "run-1"})

	if c.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after ingest, got %d", c.SubscriberCount())
	}
}

func TestBroadcastWithNoSubscribersNoPanic(t *testing.T) {
	c := New(nil)
	c.broadcast(protocol.MetricSnapshot{RunID: "run-1", OpsPerSec: 500})
}

func TestBufferLimit(t *testing.T) {
	c := New(nil)
	c.bufferSize = 5

	for i := 0; i < 20; i++ {
		c.Ingest(protocol.MetricSnapshot{
			RunID:     "run-1",
			OpsPerSec: float64(i),
		})
	}

	metrics := c.RecentMetrics("", 0)
	if len(metrics) != 5 {
		t.Fatalf("expected buffer capped at 5, got %d", len(metrics))
	}
	if metrics[0].OpsPerSec != 15 {
		t.Errorf("expected oldest buffered metric ops=15, got %f", metrics[0].OpsPerSec)
	}
	if metrics[4].OpsPerSec != 19 {
		t.Errorf("expected newest buffered metric ops=19, got %f", metrics[4].OpsPerSec)
	}
}

func TestIngestChaosEventNoPanic(t *testing.T) {
	c := New(nil)
	c.IngestChaosEvent(protocol.ChaosEvent{
		Scenario: "node_kill",
		Action:   "start",
		Target:   "crdb-1",
	})
}
