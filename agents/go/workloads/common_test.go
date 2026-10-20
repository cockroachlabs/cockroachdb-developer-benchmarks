package workloads

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestNewMetricsRecorderInitializes(t *testing.T) {
	m := NewMetricsRecorder()
	if m == nil {
		t.Fatal("NewMetricsRecorder returned nil")
	}
	if m.histogram == nil {
		t.Error("histogram not initialized")
	}
	if m.errorSamples == nil {
		t.Error("errorSamples map not initialized")
	}
	if m.opsTotal.Load() != 0 {
		t.Errorf("expected initial ops 0, got %d", m.opsTotal.Load())
	}
	if m.errors.Load() != 0 {
		t.Errorf("expected initial errors 0, got %d", m.errors.Load())
	}
}

func TestRecordOpIncrementsOpsAndRecordsLatency(t *testing.T) {
	m := NewMetricsRecorder()

	m.RecordOp(100)
	m.RecordOp(200)
	m.RecordOp(300)

	if m.opsTotal.Load() != 3 {
		t.Errorf("expected 3 ops, got %d", m.opsTotal.Load())
	}

	p50 := m.histogram.ValueAtQuantile(50)
	if p50 <= 0 {
		t.Error("expected histogram to have recorded values")
	}
}

func TestRecordErrorWithPgconnPgError(t *testing.T) {
	m := NewMetricsRecorder()

	pgErr := &pgconn.PgError{
		Code:    "40001",
		Message: "serialization failure",
	}
	m.RecordError(pgErr)

	if m.errors.Load() != 1 {
		t.Errorf("expected 1 error, got %d", m.errors.Load())
	}

	entry, exists := m.errorSamples["40001"]
	if !exists {
		t.Fatal("expected error sample with SQLSTATE 40001")
	}
	if entry.sqlstate != "40001" {
		t.Errorf("expected sqlstate 40001, got %s", entry.sqlstate)
	}
	if entry.message != "serialization failure" {
		t.Errorf("expected message 'serialization failure', got %s", entry.message)
	}
	if entry.count != 1 {
		t.Errorf("expected count 1, got %d", entry.count)
	}
}

func TestRecordErrorWithRegularError(t *testing.T) {
	m := NewMetricsRecorder()

	m.RecordError(errors.New("connection refused"))

	if m.errors.Load() != 1 {
		t.Errorf("expected 1 error, got %d", m.errors.Load())
	}

	entry, exists := m.errorSamples["UNKNOWN"]
	if !exists {
		t.Fatal("expected error sample with SQLSTATE UNKNOWN")
	}
	if entry.sqlstate != "UNKNOWN" {
		t.Errorf("expected sqlstate UNKNOWN, got %s", entry.sqlstate)
	}
}

func TestRecordErrorWithNilError(t *testing.T) {
	m := NewMetricsRecorder()

	m.RecordError(nil)

	if m.errors.Load() != 1 {
		t.Errorf("expected 1 error (nil still increments counter), got %d", m.errors.Load())
	}
	if len(m.errorSamples) != 0 {
		t.Errorf("expected no error samples for nil error, got %d", len(m.errorSamples))
	}
}

func TestSnapshotReturnsCorrectValues(t *testing.T) {
	m := NewMetricsRecorder()

	m.RecordOp(100)
	m.RecordOp(200)
	m.RecordOp(300)
	m.RecordError(errors.New("test error"))
	m.RecordRetry()
	m.RecordRetrySuccess()
	m.SetActiveConns(5)

	time.Sleep(10 * time.Millisecond)

	snap := m.Snapshot("run-1", "library", "crud")

	if snap.RunID != "run-1" {
		t.Errorf("expected RunID run-1, got %s", snap.RunID)
	}
	if snap.Language != "go" {
		t.Errorf("expected Language go, got %s", snap.Language)
	}
	if snap.Mode != "library" {
		t.Errorf("expected Mode library, got %s", snap.Mode)
	}
	if snap.Workload != "crud" {
		t.Errorf("expected Workload crud, got %s", snap.Workload)
	}
	if snap.OpsTotal != 3 {
		t.Errorf("expected OpsTotal 3, got %d", snap.OpsTotal)
	}
	if snap.Errors != 1 {
		t.Errorf("expected Errors 1, got %d", snap.Errors)
	}
	if snap.Retries != 1 {
		t.Errorf("expected Retries 1, got %d", snap.Retries)
	}
	if snap.RetrySuccess != 1 {
		t.Errorf("expected RetrySuccess 1, got %d", snap.RetrySuccess)
	}
	if snap.ActiveConnections != 5 {
		t.Errorf("expected ActiveConnections 5, got %d", snap.ActiveConnections)
	}
	if snap.Timestamp <= 0 {
		t.Error("expected positive timestamp")
	}
	if snap.MemRSSMB <= 0 {
		t.Error("expected positive memory reading")
	}
}

func TestSetWarmupPreventsRecording(t *testing.T) {
	m := NewMetricsRecorder()
	m.SetWarmup(200 * time.Millisecond)

	m.RecordOp(100)
	m.RecordOp(200)

	if m.opsTotal.Load() != 0 {
		t.Errorf("expected 0 ops during warmup, got %d", m.opsTotal.Load())
	}

	time.Sleep(250 * time.Millisecond)

	m.RecordOp(100)
	if m.opsTotal.Load() != 1 {
		t.Errorf("expected 1 op after warmup, got %d", m.opsTotal.Load())
	}
}

func TestSnapshotReportsWarmupState(t *testing.T) {
	m := NewMetricsRecorder()
	m.SetWarmup(200 * time.Millisecond)

	snap := m.Snapshot("run-1", "library", "crud")
	if !snap.Warmup {
		t.Error("expected Warmup=true during warmup period")
	}

	time.Sleep(250 * time.Millisecond)

	snap = m.Snapshot("run-1", "library", "crud")
	if snap.Warmup {
		t.Error("expected Warmup=false after warmup period")
	}
}

func TestCreateRunnerValidWorkloads(t *testing.T) {
	rec := NewMetricsRecorder()

	workloads := []string{"crud", "retry_storm", "mixed_rw", "follower_reads", "cdc", "vector_search"}
	modes := []bool{true, false}

	for _, wl := range workloads {
		for _, isLib := range modes {
			runner := CreateRunner(wl, isLib, rec)
			if runner == nil {
				t.Errorf("CreateRunner(%s, isLibrary=%v) returned nil", wl, isLib)
			}
		}
	}
}

func TestCreateRunnerInvalidWorkload(t *testing.T) {
	rec := NewMetricsRecorder()
	runner := CreateRunner("nonexistent", true, rec)
	if runner != nil {
		t.Error("expected nil runner for invalid workload type")
	}
}

func TestRecordRetryAndRetrySuccess(t *testing.T) {
	m := NewMetricsRecorder()
	m.RecordRetry()
	m.RecordRetry()
	m.RecordRetrySuccess()

	if m.retries.Load() != 2 {
		t.Errorf("expected 2 retries, got %d", m.retries.Load())
	}
	if m.retrySuccess.Load() != 1 {
		t.Errorf("expected 1 retry success, got %d", m.retrySuccess.Load())
	}
}

func TestErrorSamplesCappedAt10(t *testing.T) {
	m := NewMetricsRecorder()

	for i := 0; i < 15; i++ {
		pgErr := &pgconn.PgError{
			Code:    "CODE" + string(rune('A'+i)),
			Message: "error",
		}
		m.RecordError(pgErr)
	}

	if len(m.errorSamples) > 10 {
		t.Errorf("expected error samples capped at 10, got %d", len(m.errorSamples))
	}
}
