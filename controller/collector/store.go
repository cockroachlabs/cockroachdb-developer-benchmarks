package collector

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
	_ "modernc.org/sqlite"
)

type Store struct {
	db         *sql.DB
	resultsDir string
}

func NewStore(dbPath, resultsDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	if resultsDir != "" {
		if err := os.MkdirAll(resultsDir, 0o755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db, resultsDir: resultsDir}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY,
			timestamp TEXT NOT NULL,
			topology TEXT NOT NULL,
			workloads TEXT NOT NULL,
			duration_secs INTEGER NOT NULL,
			agents TEXT NOT NULL,
			chaos_events TEXT
		);
		CREATE TABLE IF NOT EXISTS metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			language TEXT NOT NULL,
			mode TEXT NOT NULL,
			workload TEXT NOT NULL,
			ops_total INTEGER,
			ops_per_sec REAL,
			latency_p50_us REAL,
			latency_p95_us REAL,
			latency_p99_us REAL,
			latency_max_us REAL,
			errors INTEGER,
			retries INTEGER,
			retry_success INTEGER,
			circuit_breaker_open INTEGER,
			active_connections INTEGER,
			cpu_percent REAL,
			mem_rss_mb REAL,
			FOREIGN KEY (run_id) REFERENCES runs(id)
		);
		CREATE INDEX IF NOT EXISTS idx_metrics_run ON metrics(run_id);
		CREATE INDEX IF NOT EXISTS idx_metrics_lang ON metrics(run_id, language, mode);
	`)
	return err
}

func (s *Store) SaveRun(run protocol.BenchmarkRun) error {
	workloadsJSON, _ := json.Marshal(run.Workloads)
	agentsJSON, _ := json.Marshal(run.Agents)
	chaosJSON, _ := json.Marshal(run.Chaos)

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO runs (id, timestamp, topology, workloads, duration_secs, agents, chaos_events)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.Timestamp.Format(time.RFC3339), run.Topology,
		string(workloadsJSON), run.Duration, string(agentsJSON), string(chaosJSON),
	)
	return err
}

func (s *Store) SaveMetric(m protocol.MetricSnapshot) error {
	cbOpen := 0
	if m.CircuitBreakerOpen {
		cbOpen = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO metrics (run_id, timestamp, language, mode, workload,
			ops_total, ops_per_sec, latency_p50_us, latency_p95_us, latency_p99_us, latency_max_us,
			errors, retries, retry_success, circuit_breaker_open, active_connections, cpu_percent, mem_rss_mb)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.RunID, m.Timestamp, m.Language, m.Mode, m.Workload,
		m.OpsTotal, m.OpsPerSec, m.LatencyP50Us, m.LatencyP95Us, m.LatencyP99Us, m.LatencyMaxUs,
		m.Errors, m.Retries, m.RetrySuccess, cbOpen, m.ActiveConnections, m.CPUPercent, m.MemRSSMB,
	)
	return err
}

func (s *Store) ListRuns() ([]protocol.BenchmarkRun, error) {
	rows, err := s.db.Query(`SELECT id, timestamp, topology, workloads, duration_secs, agents, chaos_events FROM runs ORDER BY timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]protocol.BenchmarkRun, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return runs, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) GetRun(id string) (*protocol.BenchmarkRun, error) {
	row := s.db.QueryRow(
		`SELECT id, timestamp, topology, workloads, duration_secs, agents, chaos_events FROM runs WHERE id = ?`, id,
	)

	var tsStr, topoStr, workloadsStr, agentsStr, chaosStr string
	var duration int
	var runID string
	err := row.Scan(&runID, &tsStr, &topoStr, &workloadsStr, &duration, &agentsStr, &chaosStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339, tsStr)
	var workloads []protocol.WorkloadType
	json.Unmarshal([]byte(workloadsStr), &workloads)
	var agents []protocol.AgentEntry
	json.Unmarshal([]byte(agentsStr), &agents)
	var chaos []protocol.ChaosEvent
	json.Unmarshal([]byte(chaosStr), &chaos)

	return &protocol.BenchmarkRun{
		ID:        runID,
		Timestamp: ts,
		Topology:  protocol.Topology(topoStr),
		Workloads: workloads,
		Duration:  duration,
		Agents:    agents,
		Chaos:     chaos,
	}, nil
}

func (s *Store) GetRunMetrics(runID string) ([]protocol.MetricSnapshot, error) {
	rows, err := s.db.Query(
		`SELECT timestamp, run_id, language, mode, workload,
			ops_total, ops_per_sec, latency_p50_us, latency_p95_us, latency_p99_us, latency_max_us,
			errors, retries, retry_success, circuit_breaker_open, active_connections, cpu_percent, mem_rss_mb
		 FROM metrics WHERE run_id = ? ORDER BY timestamp`, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []protocol.MetricSnapshot
	for rows.Next() {
		var m protocol.MetricSnapshot
		var cbOpen int
		err := rows.Scan(
			&m.Timestamp, &m.RunID, &m.Language, &m.Mode, &m.Workload,
			&m.OpsTotal, &m.OpsPerSec, &m.LatencyP50Us, &m.LatencyP95Us, &m.LatencyP99Us, &m.LatencyMaxUs,
			&m.Errors, &m.Retries, &m.RetrySuccess, &cbOpen, &m.ActiveConnections, &m.CPUPercent, &m.MemRSSMB,
		)
		if err != nil {
			return nil, err
		}
		m.CircuitBreakerOpen = cbOpen == 1
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

func (s *Store) ExportRun(runID string) ([]byte, error) {
	run, err := s.GetRun(runID)
	if err != nil || run == nil {
		return nil, fmt.Errorf("run %s not found", runID)
	}

	metrics, err := s.GetRunMetrics(runID)
	if err != nil {
		return nil, err
	}

	export := map[string]interface{}{
		"run":     run,
		"metrics": metrics,
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, err
	}

	if s.resultsDir != "" {
		path := filepath.Join(s.resultsDir, fmt.Sprintf("%s.json", runID))
		os.WriteFile(path, data, 0o644)
	}

	return data, nil
}

func (s *Store) DeleteRun(id string) error {
	_, err := s.db.Exec("DELETE FROM metrics WHERE run_id = ?", id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM runs WHERE id = ?", id)
	return err
}

func (s *Store) DeleteAllRuns() error {
	_, err := s.db.Exec("DELETE FROM metrics")
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM runs")
	return err
}

type TrendPoint struct {
	RunID     string  `json:"run_id"`
	Timestamp string  `json:"timestamp"`
	Language  string  `json:"language"`
	Mode      string  `json:"mode"`
	Workload  string  `json:"workload"`
	AvgOps    float64 `json:"avg_ops"`
}

func (s *Store) GetTrends(limit int) ([]TrendPoint, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT r.id, r.timestamp, m.language, m.mode, m.workload, AVG(m.ops_per_sec) as avg_ops
		FROM metrics m
		JOIN runs r ON r.id = m.run_id
		WHERE m.ops_per_sec > 0
		GROUP BY r.id, r.timestamp, m.language, m.mode, m.workload
		ORDER BY r.timestamp DESC
		LIMIT ?`, limit*50)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TrendPoint
	for rows.Next() {
		var p TrendPoint
		if err := rows.Scan(&p.RunID, &p.Timestamp, &p.Language, &p.Mode, &p.Workload, &p.AvgOps); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

type RegressionItem struct {
	Agent         string  `json:"agent"`
	Workload      string  `json:"workload"`
	Metric        string  `json:"metric"`
	PreviousValue float64 `json:"previous_value"`
	CurrentValue  float64 `json:"current_value"`
	ChangePct     float64 `json:"change_pct"`
	IsRegression  bool    `json:"is_regression"`
}

func (s *Store) GetRegressionData() ([]RegressionItem, error) {
	runs, err := s.ListRuns()
	if err != nil || len(runs) < 2 {
		return nil, err
	}
	currentID := runs[0].ID
	previousID := runs[1].ID

	currentMetrics, err := s.GetRunMetrics(currentID)
	if err != nil {
		return nil, err
	}
	previousMetrics, err := s.GetRunMetrics(previousID)
	if err != nil {
		return nil, err
	}

	type agentWorkloadKey struct {
		agent    string
		workload string
	}
	type agg struct {
		opsSum float64
		p99Sum float64
		count  int
	}

	aggregate := func(metrics []protocol.MetricSnapshot) map[agentWorkloadKey]*agg {
		m := map[agentWorkloadKey]*agg{}
		for _, snap := range metrics {
			if snap.OpsPerSec <= 0 {
				continue
			}
			k := agentWorkloadKey{agent: snap.Language + "/" + snap.Mode, workload: snap.Workload}
			a, ok := m[k]
			if !ok {
				a = &agg{}
				m[k] = a
			}
			a.opsSum += snap.OpsPerSec
			a.p99Sum += snap.LatencyP99Us
			a.count++
		}
		return m
	}

	curr := aggregate(currentMetrics)
	prev := aggregate(previousMetrics)

	var items []RegressionItem
	for k, prevAgg := range prev {
		currAgg, ok := curr[k]
		if !ok || prevAgg.count == 0 {
			continue
		}
		prevOps := prevAgg.opsSum / float64(prevAgg.count)
		currOps := currAgg.opsSum / float64(currAgg.count)
		if prevOps > 0 {
			changePct := ((currOps - prevOps) / prevOps) * 100
			items = append(items, RegressionItem{
				Agent:         k.agent,
				Workload:      k.workload,
				Metric:        "ops_per_sec",
				PreviousValue: prevOps,
				CurrentValue:  currOps,
				ChangePct:     changePct,
				IsRegression:  changePct < -10,
			})
		}

		prevP99 := prevAgg.p99Sum / float64(prevAgg.count)
		currP99 := currAgg.p99Sum / float64(currAgg.count)
		if prevP99 > 0 {
			changePct := ((currP99 - prevP99) / prevP99) * 100
			items = append(items, RegressionItem{
				Agent:         k.agent,
				Workload:      k.workload,
				Metric:        "latency_p99_us",
				PreviousValue: prevP99,
				CurrentValue:  currP99,
				ChangePct:     changePct,
				IsRegression:  changePct > 20,
			})
		}
	}
	return items, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func scanRun(rows *sql.Rows) (protocol.BenchmarkRun, error) {
	var run protocol.BenchmarkRun
	var tsStr, topoStr, workloadsStr, agentsStr, chaosStr string

	err := rows.Scan(&run.ID, &tsStr, &topoStr, &workloadsStr, &run.Duration, &agentsStr, &chaosStr)
	if err != nil {
		return run, err
	}

	run.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
	run.Topology = protocol.Topology(topoStr)
	json.Unmarshal([]byte(workloadsStr), &run.Workloads)
	json.Unmarshal([]byte(agentsStr), &run.Agents)
	json.Unmarshal([]byte(chaosStr), &run.Chaos)
	return run, nil
}
