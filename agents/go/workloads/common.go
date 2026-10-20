package workloads

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	hdr "github.com/HdrHistogram/hdrhistogram-go"
	"github.com/jackc/pgx/v5/pgconn"
)

type WorkloadConfig struct {
	DurationSecs int             `json:"duration_secs"`
	Concurrency  int             `json:"concurrency"`
	DBURL        string          `json:"db_url"`
	HotRows      int             `json:"hot_rows,omitempty"`
	WarmupSecs   int             `json:"warmup_secs,omitempty"`
	ScenarioData json.RawMessage `json:"scenario_data,omitempty"`
}

type ErrorSample struct {
	SQLState string `json:"sqlstate"`
	Message  string `json:"message"`
	Count    int64  `json:"count"`
}

type MetricSnapshot struct {
	Timestamp          int64          `json:"ts"`
	RunID              string         `json:"run_id"`
	Language           string         `json:"language"`
	Mode               string         `json:"mode"`
	Workload           string         `json:"workload"`
	OpsTotal           int64          `json:"ops_total"`
	OpsPerSec          float64        `json:"ops_per_sec"`
	LatencyP50Us       float64        `json:"latency_p50_us"`
	LatencyP95Us       float64        `json:"latency_p95_us"`
	LatencyP99Us       float64        `json:"latency_p99_us"`
	LatencyMaxUs       float64        `json:"latency_max_us"`
	Errors             int64          `json:"errors"`
	Retries            int64          `json:"retries"`
	RetrySuccess       int64          `json:"retry_success"`
	CircuitBreakerOpen bool           `json:"circuit_breaker_open"`
	ActiveConnections  int            `json:"active_connections"`
	CPUPercent         float64        `json:"cpu_percent"`
	MemRSSMB           float64        `json:"mem_rss_mb"`
	ErrorSamples       []ErrorSample  `json:"error_samples,omitempty"`
	Warmup             bool           `json:"warmup,omitempty"`
}

type WorkloadRunner interface {
	Start(ctx context.Context, config WorkloadConfig) error
	Stop()
}

type errorEntry struct {
	sqlstate string
	message  string
	count    int64
}

type MetricsRecorder struct {
	mu           sync.Mutex
	histogram    *hdr.Histogram
	opsTotal     atomic.Int64
	errors       atomic.Int64
	retries      atomic.Int64
	retrySuccess atomic.Int64
	lastOps      int64
	lastTime     time.Time
	activeConns  atomic.Int32
	startCPU     time.Duration
	errorSamples map[string]*errorEntry
	warmupUntil  time.Time
}

func NewMetricsRecorder() *MetricsRecorder {
	m := &MetricsRecorder{
		histogram:    hdr.New(1, 60_000_000, 3),
		lastTime:     time.Now(),
		errorSamples: make(map[string]*errorEntry),
	}
	var rusage runtime.MemStats
	runtime.ReadMemStats(&rusage)
	return m
}

func (m *MetricsRecorder) SetWarmup(duration time.Duration) {
	m.mu.Lock()
	m.warmupUntil = time.Now().Add(duration)
	m.mu.Unlock()
}

func (m *MetricsRecorder) RecordOp(latencyUs int64) {
	if !m.warmupUntil.IsZero() && time.Now().Before(m.warmupUntil) {
		return
	}
	m.opsTotal.Add(1)
	m.mu.Lock()
	m.histogram.RecordValue(latencyUs)
	m.mu.Unlock()
}

func (m *MetricsRecorder) RecordError(err error) {
	m.errors.Add(1)
	if err == nil {
		return
	}

	sqlstate := "UNKNOWN"
	msg := err.Error()
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		sqlstate = pgErr.Code
		msg = pgErr.Message
	}

	m.mu.Lock()
	if len(m.errorSamples) < 10 {
		entry, exists := m.errorSamples[sqlstate]
		if !exists {
			m.errorSamples[sqlstate] = &errorEntry{sqlstate: sqlstate, message: msg, count: 1}
		} else {
			entry.count++
		}
	} else {
		if entry, exists := m.errorSamples[sqlstate]; exists {
			entry.count++
		}
	}
	m.mu.Unlock()
}

func (m *MetricsRecorder) RecordRetry() {
	m.retries.Add(1)
}

func (m *MetricsRecorder) RecordRetrySuccess() {
	m.retrySuccess.Add(1)
}

func (m *MetricsRecorder) SetActiveConns(n int32) {
	m.activeConns.Store(n)
}

func (m *MetricsRecorder) Snapshot(runID, mode, workload string) MetricSnapshot {
	now := time.Now()
	currentOps := m.opsTotal.Load()
	elapsed := now.Sub(m.lastTime).Seconds()
	opsPerSec := 0.0
	if elapsed > 0 {
		opsPerSec = float64(currentOps-m.lastOps) / elapsed
	}

	m.mu.Lock()
	p50 := float64(m.histogram.ValueAtQuantile(50))
	p95 := float64(m.histogram.ValueAtQuantile(95))
	p99 := float64(m.histogram.ValueAtQuantile(99))
	maxVal := float64(m.histogram.Max())
	m.histogram.Reset()

	var samples []ErrorSample
	for _, e := range m.errorSamples {
		samples = append(samples, ErrorSample{
			SQLState: e.sqlstate,
			Message:  e.message,
			Count:    e.count,
		})
	}
	m.mu.Unlock()

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Count > samples[j].Count
	})

	m.lastOps = currentOps
	m.lastTime = now

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	cpuPercent := measureCPU()

	inWarmup := !m.warmupUntil.IsZero() && now.Before(m.warmupUntil)

	snap := MetricSnapshot{
		Timestamp:         now.UnixMilli(),
		RunID:             runID,
		Language:          "go",
		Mode:              mode,
		Workload:          workload,
		OpsTotal:          currentOps,
		OpsPerSec:         opsPerSec,
		LatencyP50Us:      p50,
		LatencyP95Us:      p95,
		LatencyP99Us:      p99,
		LatencyMaxUs:      maxVal,
		Errors:            m.errors.Load(),
		Retries:           m.retries.Load(),
		RetrySuccess:      m.retrySuccess.Load(),
		ActiveConnections: int(m.activeConns.Load()),
		CPUPercent:        cpuPercent,
		MemRSSMB:          float64(memStats.Sys) / 1024 / 1024,
		Warmup:            inWarmup,
	}
	if len(samples) > 0 {
		snap.ErrorSamples = samples
	}
	return snap
}

var (
	lastCPUTime  time.Time
	lastCPUUsage time.Duration
	cpuMu        sync.Mutex
)

func measureCPU() float64 {
	cpuMu.Lock()
	defer cpuMu.Unlock()

	now := time.Now()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if lastCPUTime.IsZero() {
		lastCPUTime = now
		return 0
	}

	wall := now.Sub(lastCPUTime).Seconds()
	lastCPUTime = now
	if wall <= 0 {
		return 0
	}

	numCPU := float64(runtime.NumCPU())
	goroutines := float64(runtime.NumGoroutine())
	estimate := (goroutines / (numCPU * 100)) * 100
	if estimate > 100 {
		estimate = 100
	}
	return estimate
}

func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
