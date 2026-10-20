package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/chaos"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/collector"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/report"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/scenario"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/scheduler"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/topology"
	"github.com/gorilla/websocket"
)

// spaHandler serves static files from dir, falling back to index.html for
// paths that don't map to a real file so client-side routes (/live, /compare,
// …) load the single-page app instead of returning 404.
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}

type Handler struct {
	topo         *topology.Manager
	sched        *scheduler.Scheduler
	chaos        *chaos.Engine
	collector    *collector.Collector
	store        *collector.Store
	upgrader     websocket.Upgrader
	dashDir      string
	scenariosDir string
}

func NewHandler(
	topo *topology.Manager,
	sched *scheduler.Scheduler,
	chaosEngine *chaos.Engine,
	coll *collector.Collector,
	store *collector.Store,
	dashDir string,
	scenariosDir string,
) *Handler {
	return &Handler{
		topo:         topo,
		sched:        sched,
		chaos:        chaosEngine,
		collector:    coll,
		store:        store,
		dashDir:      dashDir,
		scenariosDir: scenariosDir,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", h.handleStatus)
	mux.HandleFunc("POST /api/benchmark/start", h.handleStart)
	mux.HandleFunc("POST /api/benchmark/stop", h.handleStop)
	mux.HandleFunc("POST /api/topology/start", h.handleTopoStart)
	mux.HandleFunc("POST /api/topology/stop", h.handleTopoStop)
	mux.HandleFunc("POST /api/teardown", h.handleTeardown)
	mux.HandleFunc("GET /api/runs", h.handleListRuns)
	mux.HandleFunc("DELETE /api/runs", h.handleDeleteAllRuns)
	mux.HandleFunc("POST /api/runs/compare/report", h.handleCompareReport)
	mux.HandleFunc("GET /api/runs/", h.handleRunRoutes)
	mux.HandleFunc("DELETE /api/runs/", h.handleDeleteRun)
	mux.HandleFunc("/ws/metrics", h.handleMetricsWS)
	mux.HandleFunc("GET /metrics", h.handlePrometheus)
	mux.HandleFunc("POST /api/monitoring/start", h.handleMonitoringStart)
	mux.HandleFunc("POST /api/monitoring/stop", h.handleMonitoringStop)
	mux.HandleFunc("POST /api/chaos/trigger", h.handleChaosTrigger)
	mux.HandleFunc("GET /api/chaos/status", h.handleChaosStatus)
	mux.HandleFunc("GET /api/chaos/plans", h.handleChaosPlans)
	mux.HandleFunc("POST /api/chaos/plan", h.handleChaosRunPlan)
	mux.HandleFunc("GET /api/trends", h.handleTrends)
	mux.HandleFunc("GET /api/regression", h.handleRegression)
	mux.HandleFunc("GET /api/scenarios", h.handleListScenarios)
	mux.HandleFunc("POST /api/scenario/run", h.handleRunScenario)
	mux.HandleFunc("POST /api/scenarios/upload", h.handleUploadScenario)

	if h.dashDir != "" {
		mux.Handle("/", spaHandler(h.dashDir))
	}
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":      "ok",
		"topology":    h.topo.Active(),
		"topo_running": h.topo.Running(),
		"benchmark":   h.sched.IsRunning(),
		"chaos":       h.chaos.IsRunning(),
		"subscribers": h.collector.SubscriberCount(),
	}

	if run := h.sched.CurrentRun(); run != nil {
		status["current_run"] = map[string]interface{}{
			"id":         run.ID,
			"started_at": run.StartedAt,
		}
	}

	writeJSON(w, http.StatusOK, status)
}

type StartRequest struct {
	Workloads      []protocol.WorkloadType    `json:"workloads"`
	Topology       protocol.Topology          `json:"topology"`
	DurationSecs   int                        `json:"duration_secs"`
	Duration       int                        `json:"duration"`
	Concurrency    int                        `json:"concurrency,omitempty"`
	WarmupSecs     int                        `json:"warmup_secs,omitempty"`
	Iterations     int                        `json:"iterations,omitempty"`
	Languages      []string                   `json:"languages,omitempty"`
	Isolation      bool                       `json:"isolation,omitempty"`
	ChaosMode      protocol.ChaosMode         `json:"chaos_mode,omitempty"`
	ChaosScenarios []protocol.ChaosScenario   `json:"chaos_scenarios,omitempty"`
}

func (h *Handler) handleStart(w http.ResponseWriter, r *http.Request) {
	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if len(req.Workloads) == 0 {
		req.Workloads = []protocol.WorkloadType{
			protocol.WorkloadCRUD,
			protocol.WorkloadRetryStorm,
			protocol.WorkloadMixedRW,
			protocol.WorkloadFollowerReads,
			protocol.WorkloadCDC,
			protocol.WorkloadVectorSearch,
		}
	}

	if req.DurationSecs == 0 && req.Duration > 0 {
		req.DurationSecs = req.Duration
	}
	if req.DurationSecs == 0 {
		req.DurationSecs = 60
	}

	if req.Topology != "" && req.Topology != h.topo.Active() {
		if h.topo.Running() {
			if err := h.topo.Stop(r.Context()); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		if err := h.topo.Start(r.Context(), req.Topology); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	if req.Concurrency == 0 {
		req.Concurrency = 8
	}

	if req.WarmupSecs == 0 {
		req.WarmupSecs = 10
	}

	if req.Iterations <= 0 {
		req.Iterations = 1
	}
	if req.Iterations > 10 {
		req.Iterations = 10
	}

	agents := protocol.DefaultAgents
	if len(req.Languages) > 0 {
		langSet := make(map[string]bool, len(req.Languages))
		for _, l := range req.Languages {
			langSet[l] = true
		}
		filtered := make([]protocol.AgentEntry, 0, len(agents))
		for _, a := range agents {
			if langSet[string(a.Language)] {
				filtered = append(filtered, a)
			}
		}
		agents = filtered
	}

	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	run := protocol.BenchmarkRun{
		ID:          runID,
		Timestamp:   time.Now(),
		Topology:    h.topo.Active(),
		Workloads:   req.Workloads,
		Duration:    req.DurationSecs,
		Concurrency: req.Concurrency,
		WarmupSecs:  req.WarmupSecs,
		Iterations:  req.Iterations,
		Languages:   req.Languages,
		Isolation:   req.Isolation,
		Agents:      agents,
	}

	if h.store != nil {
		h.store.SaveRun(run)
	}

	if err := h.sched.StartRun(context.Background(), run); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	if req.ChaosMode != "" && h.topo.Active() == protocol.TopoMulti {
		if err := h.chaos.Start(context.Background(), req.ChaosMode, req.ChaosScenarios); err != nil {
			log.Printf("chaos start warning: %v", err)
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"run_id": runID,
		"status": "started",
	})
}

func (h *Handler) handleStop(w http.ResponseWriter, r *http.Request) {
	h.chaos.Stop()

	if err := h.sched.StopRun(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handler) handleTopoStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Topology protocol.Topology `json:"topology"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Topology == "" {
		req.Topology = protocol.TopoSingle
	}

	if h.topo.Running() {
		if err := h.topo.Stop(context.Background()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	if err := h.topo.Start(context.Background(), req.Topology); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "started",
		"topology": string(req.Topology),
	})
}

func (h *Handler) handleTeardown(w http.ResponseWriter, r *http.Request) {
	results := map[string]string{}

	h.chaos.Stop()
	h.sched.StopRun()
	results["benchmark"] = "stopped"

	h.sched.StopAllAgents(context.Background())
	results["agents"] = "stopped"

	if h.topo.Running() {
		if err := h.topo.Stop(context.Background()); err != nil {
			results["topology"] = "error: " + err.Error()
		} else {
			results["topology"] = "stopped"
		}
	} else {
		results["topology"] = "not running"
	}

	results["status"] = "torn down"
	writeJSON(w, http.StatusOK, results)
}

func (h *Handler) handleTopoStop(w http.ResponseWriter, r *http.Request) {
	if !h.topo.Running() {
		writeJSON(w, http.StatusOK, map[string]string{"status": "not running"})
		return
	}

	if err := h.topo.Stop(context.Background()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handler) handleDeleteAllRuns(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no store"})
		return
	}
	if err := h.store.DeleteAllRuns(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	if h.store == nil || runID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := h.store.DeleteRun(runID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) handleListRuns(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	runs, err := h.store.ListRuns()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, runs)
}

func (h *Handler) handleRunRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		h.handleListRuns(w, r)
		return
	}

	runID := parts[0]

	if len(parts) >= 2 && parts[1] == "export" {
		h.handleExport(w, r, runID)
		return
	}

	if len(parts) >= 2 && parts[1] == "report" {
		download := len(parts) >= 3 && parts[2] == "download"
		h.handleReport(w, r, runID, download)
		return
	}

	h.handleGetRun(w, r, runID)
}

func (h *Handler) handleGetRun(w http.ResponseWriter, r *http.Request, runID string) {
	if h.store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store"})
		return
	}

	run, err := h.store.GetRun(runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}

	metrics, _ := h.store.GetRunMetrics(runID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"run":     run,
		"metrics": metrics,
	})
}

func (h *Handler) handleExport(w http.ResponseWriter, r *http.Request, runID string) {
	if h.store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store"})
		return
	}

	data, err := h.store.ExportRun(runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", runID))
	w.Write(data)
}

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request, runID string, download bool) {
	if h.store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store"})
		return
	}

	resolvedID := runID
	if runID == "latest" {
		runs, err := h.store.ListRuns()
		if err != nil || len(runs) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no runs found"})
			return
		}
		resolvedID = runs[0].ID
	}

	run, err := h.store.GetRun(resolvedID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}

	metrics, _ := h.store.GetRunMetrics(resolvedID)

	chaosEvents := run.Chaos

	var previousMetrics []protocol.MetricSnapshot
	runs, _ := h.store.ListRuns()
	for i, r := range runs {
		if r.ID == resolvedID && i+1 < len(runs) {
			previousMetrics, _ = h.store.GetRunMetrics(runs[i+1].ID)
			break
		}
	}

	html, err := report.GenerateHTML(*run, metrics, chaosEvents, previousMetrics)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if download {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s-report.html", resolvedID))
	}
	w.Write(html)
}

func (h *Handler) handleCompareReport(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store"})
		return
	}

	var req struct {
		RunIDs []string `json:"run_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(req.RunIDs) < 2 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least 2 run_ids required"})
		return
	}

	var runs []protocol.BenchmarkRun
	var allMetrics [][]protocol.MetricSnapshot
	for _, id := range req.RunIDs {
		run, err := h.store.GetRun(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if run == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + id})
			return
		}
		runs = append(runs, *run)
		metrics, _ := h.store.GetRunMetrics(id)
		allMetrics = append(allMetrics, metrics)
	}

	html, err := report.GenerateComparisonHTML(runs, allMetrics)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}

func (h *Handler) handleMetricsWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	h.collector.Subscribe(conn)
	defer h.collector.Unsubscribe(conn)

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *Handler) handlePrometheus(w http.ResponseWriter, _ *http.Request) {
	recent := h.collector.RecentMetrics("", 100)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintln(w, "# HELP crdb_benchmark_ops_per_sec Operations per second")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_ops_per_sec gauge")
	fmt.Fprintln(w, "# HELP crdb_benchmark_latency_p50_us P50 latency in microseconds")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_latency_p50_us gauge")
	fmt.Fprintln(w, "# HELP crdb_benchmark_latency_p95_us P95 latency in microseconds")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_latency_p95_us gauge")
	fmt.Fprintln(w, "# HELP crdb_benchmark_latency_p99_us P99 latency in microseconds")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_latency_p99_us gauge")
	fmt.Fprintln(w, "# HELP crdb_benchmark_errors_total Total errors")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_errors_total counter")
	fmt.Fprintln(w, "# HELP crdb_benchmark_retries_total Total retries")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_retries_total counter")
	fmt.Fprintln(w, "# HELP crdb_benchmark_ops_total Total operations")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_ops_total counter")
	fmt.Fprintln(w, "# HELP crdb_benchmark_cpu_percent CPU usage percent")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_cpu_percent gauge")
	fmt.Fprintln(w, "# HELP crdb_benchmark_mem_rss_mb Memory RSS in MB")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_mem_rss_mb gauge")
	fmt.Fprintln(w, "# HELP crdb_benchmark_active_connections Active database connections")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_active_connections gauge")

	seen := map[string]bool{}
	for i := len(recent) - 1; i >= 0; i-- {
		m := recent[i]
		key := m.Language + "/" + m.Mode + "/" + m.Workload
		if seen[key] {
			continue
		}
		seen[key] = true

		labels := fmt.Sprintf(`language="%s",mode="%s",workload="%s"`, m.Language, m.Mode, m.Workload)
		fmt.Fprintf(w, "crdb_benchmark_ops_per_sec{%s} %f\n", labels, m.OpsPerSec)
		fmt.Fprintf(w, "crdb_benchmark_latency_p50_us{%s} %f\n", labels, m.LatencyP50Us)
		fmt.Fprintf(w, "crdb_benchmark_latency_p95_us{%s} %f\n", labels, m.LatencyP95Us)
		fmt.Fprintf(w, "crdb_benchmark_latency_p99_us{%s} %f\n", labels, m.LatencyP99Us)
		fmt.Fprintf(w, "crdb_benchmark_errors_total{%s} %d\n", labels, m.Errors)
		fmt.Fprintf(w, "crdb_benchmark_retries_total{%s} %d\n", labels, m.Retries)
		fmt.Fprintf(w, "crdb_benchmark_ops_total{%s} %d\n", labels, m.OpsTotal)
		fmt.Fprintf(w, "crdb_benchmark_cpu_percent{%s} %f\n", labels, m.CPUPercent)
		fmt.Fprintf(w, "crdb_benchmark_mem_rss_mb{%s} %f\n", labels, m.MemRSSMB)
		fmt.Fprintf(w, "crdb_benchmark_active_connections{%s} %d\n", labels, m.ActiveConnections)
	}

	fmt.Fprintln(w, "# HELP crdb_benchmark_running Whether a benchmark is currently running")
	fmt.Fprintln(w, "# TYPE crdb_benchmark_running gauge")
	running := 0
	if h.sched.IsRunning() {
		running = 1
	}
	fmt.Fprintf(w, "crdb_benchmark_running %d\n", running)
}

func (h *Handler) runCompose(args ...string) (string, error) {
	bin := "docker-compose"
	if _, err := exec.LookPath("docker-compose"); err != nil {
		bin = "docker"
		args = append([]string{"compose"}, args...)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = h.topo.DeployDir()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (h *Handler) handleMonitoringStart(w http.ResponseWriter, _ *http.Request) {
	out, err := h.runCompose("-f", "docker-compose.monitoring.yml", "-p", "crdb-monitoring", "up", "-d")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error(), "output": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *Handler) handleMonitoringStop(w http.ResponseWriter, _ *http.Request) {
	out, err := h.runCompose("-f", "docker-compose.monitoring.yml", "-p", "crdb-monitoring", "down")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error(), "output": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handler) handleChaosTrigger(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario protocol.ChaosScenario `json:"scenario"`
		Target   string                 `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Scenario == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scenario is required"})
		return
	}

	if err := h.chaos.TriggerSingle(r.Context(), req.Scenario, req.Target); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":   "triggered",
		"scenario": string(req.Scenario),
		"target":   req.Target,
	})
}

func (h *Handler) handleChaosStatus(w http.ResponseWriter, _ *http.Request) {
	events := h.chaos.Events()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running": h.chaos.IsRunning(),
		"events":  events,
	})
}

type ChaosPlan struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Events      []ChaosPlanEvent `json:"events"`
}

type ChaosPlanEvent struct {
	AtSeconds       int                    `json:"at_seconds"`
	Scenario        protocol.ChaosScenario `json:"scenario"`
	Target          string                 `json:"target,omitempty"`
	DurationSeconds int                    `json:"duration_seconds,omitempty"`
}

func (h *Handler) handleChaosPlans(w http.ResponseWriter, _ *http.Request) {
	plansDir := filepath.Join(h.topo.DeployDir(), "chaos-plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	var plans []ChaosPlan
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(plansDir, entry.Name()))
		if err != nil {
			continue
		}
		var plan ChaosPlan
		if err := json.Unmarshal(data, &plan); err != nil {
			continue
		}
		plans = append(plans, plan)
	}

	writeJSON(w, http.StatusOK, plans)
}

func (h *Handler) handleChaosRunPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Plan string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Plan == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan name is required"})
		return
	}

	plansDir := filepath.Join(h.topo.DeployDir(), "chaos-plans")
	data, err := os.ReadFile(filepath.Join(plansDir, req.Plan+".json"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found: " + req.Plan})
		return
	}

	var plan ChaosPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid plan JSON: " + err.Error()})
		return
	}

	if h.topo.Active() != protocol.TopoMulti {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "chaos plans require multi-region topology"})
		return
	}

	go func() {
		ctx := context.Background()
		start := time.Now()
		for _, ev := range plan.Events {
			wait := time.Duration(ev.AtSeconds)*time.Second - time.Since(start)
			if wait > 0 {
				time.Sleep(wait)
			}
			if err := h.chaos.TriggerSingle(ctx, ev.Scenario, ev.Target); err != nil {
				log.Printf("chaos plan %s: trigger %s failed: %v", plan.Name, ev.Scenario, err)
			}
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "plan_started",
		"plan":   plan.Name,
		"events": fmt.Sprintf("%d", len(plan.Events)),
	})
}

func (h *Handler) handleTrends(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	limit := 10
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := fmt.Sscanf(q, "%d", &limit); n != 1 || err != nil {
			limit = 10
		}
	}
	trends, err := h.store.GetTrends(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, trends)
}

func (h *Handler) handleRegression(w http.ResponseWriter, _ *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	items, err := h.store.GetRegressionData()
	if err != nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type ScenarioListItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Duration    int    `json:"duration_secs"`
	Operations  int    `json:"operations"`
}

func (h *Handler) handleListScenarios(w http.ResponseWriter, _ *http.Request) {
	var items []ScenarioListItem

	for _, s := range scenario.BuiltinScenarios {
		items = append(items, ScenarioListItem{
			Name:        s.Name,
			Description: s.Description,
			Source:      "builtin",
			Duration:    s.DurationSecs,
			Operations:  len(s.Operations),
		})
	}

	builtinNames := map[string]bool{
		"CRUD": true, "Retry Storm": true, "Mixed R/W": true,
		"Follower Reads": true, "CDC": true, "Vector Search": true,
	}

	if h.scenariosDir != "" {
		loaded, _ := scenario.LoadDir(h.scenariosDir)
		for _, s := range loaded {
			source := "file"
			if builtinNames[s.Name] {
				source = "builtin"
			}
			items = append(items, ScenarioListItem{
				Name:        s.Name,
				Description: s.Description,
				Source:      source,
				Duration:    s.DurationSecs,
				Operations:  len(s.Operations),
			})
		}

		customDir := filepath.Join(h.scenariosDir, "custom")
		custom, _ := scenario.LoadDir(customDir)
		for _, s := range custom {
			items = append(items, ScenarioListItem{
				Name:        s.Name,
				Description: s.Description,
				Source:      "custom",
				Duration:    s.DurationSecs,
				Operations:  len(s.Operations),
			})
		}
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleRunScenario(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario     string             `json:"scenario"`
		ScenarioData *scenario.Scenario `json:"scenario_data,omitempty"`
		Topology     protocol.Topology  `json:"topology,omitempty"`
		Languages    []string           `json:"languages,omitempty"`
		Iterations   int                `json:"iterations,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var sc *scenario.Scenario

	if req.ScenarioData != nil {
		if err := scenario.Validate(req.ScenarioData); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		sc = req.ScenarioData
	} else if req.Scenario != "" {
		sc = h.findScenario(req.Scenario)
		if sc == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "scenario not found: " + req.Scenario})
			return
		}
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "either 'scenario' name or 'scenario_data' is required"})
		return
	}

	if req.Topology != "" && req.Topology != h.topo.Active() {
		if h.topo.Running() {
			if err := h.topo.Stop(r.Context()); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		if err := h.topo.Start(r.Context(), req.Topology); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	scenarioJSON, err := json.Marshal(sc)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	agents := protocol.DefaultAgents
	if len(req.Languages) > 0 {
		langSet := make(map[string]bool, len(req.Languages))
		for _, l := range req.Languages {
			langSet[l] = true
		}
		filtered := make([]protocol.AgentEntry, 0, len(agents))
		for _, a := range agents {
			if langSet[string(a.Language)] {
				filtered = append(filtered, a)
			}
		}
		agents = filtered
	}

	iterations := req.Iterations
	if iterations <= 0 {
		iterations = 1
	}
	if iterations > 10 {
		iterations = 10
	}

	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	run := protocol.BenchmarkRun{
		ID:          runID,
		Timestamp:   time.Now(),
		Topology:    h.topo.Active(),
		Workloads:   []protocol.WorkloadType{protocol.WorkloadType("scenario")},
		Duration:    sc.DurationSecs,
		WarmupSecs:  sc.WarmupSecs,
		Iterations:  iterations,
		Languages:   req.Languages,
		Agents:      agents,
	}

	run.ScenarioJSON = scenarioJSON

	if h.store != nil {
		h.store.SaveRun(run)
	}

	if err := h.sched.StartRun(context.Background(), run); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"run_id":   runID,
		"status":   "started",
		"scenario": sc.Name,
	})
}

func (h *Handler) findScenario(name string) *scenario.Scenario {
	normalized := strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	normalized = strings.ReplaceAll(normalized, " ", "_")

	for _, s := range scenario.BuiltinScenarios {
		bn := strings.ToLower(strings.ReplaceAll(s.Name, "-", "_"))
		bn = strings.ReplaceAll(bn, " ", "_")
		if bn == normalized {
			return s
		}
	}

	if h.scenariosDir != "" {
		allDirs := []string{h.scenariosDir, filepath.Join(h.scenariosDir, "custom")}
		for _, dir := range allDirs {
			loaded, _ := scenario.LoadDir(dir)
			for _, s := range loaded {
				sn := strings.ToLower(strings.ReplaceAll(s.Name, "-", "_"))
				sn = strings.ReplaceAll(sn, " ", "_")
				if sn == normalized {
					return s
				}
			}
		}
	}
	return nil
}

func (h *Handler) handleUploadScenario(w http.ResponseWriter, r *http.Request) {
	if h.scenariosDir == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scenarios directory not configured"})
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if len(data) > 1<<20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scenario file too large (max 1MB)"})
		return
	}

	sc, err := scenario.Parse(data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	customDir := filepath.Join(h.scenariosDir, "custom")
	os.MkdirAll(customDir, 0755)

	safeName := strings.ReplaceAll(strings.ToLower(sc.Name), " ", "-")
	filename := filepath.Join(customDir, safeName+".json")

	if err := os.WriteFile(filename, data, 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status":   "uploaded",
		"scenario": sc.Name,
		"file":     filename,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
