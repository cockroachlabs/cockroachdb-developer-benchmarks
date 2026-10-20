package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/agents/go/workloads"
	"github.com/gorilla/websocket"
)

type WorkloadRequest struct {
	Type   string                 `json:"type"`
	Mode   string                 `json:"mode"`
	Config workloads.WorkloadConfig `json:"config"`
}

type server struct {
	mu        sync.Mutex
	mode      string
	port      string
	runner    workloads.WorkloadRunner
	recorder  *workloads.MetricsRecorder
	cancel    context.CancelFunc
	runID     string
	workload  string
	running   bool
	upgrader  websocket.Upgrader
	metricSub []chan workloads.MetricSnapshot
	subMu     sync.Mutex
}

func main() {
	mode := os.Getenv("AGENT_MODE")
	if mode == "" {
		mode = "vanilla"
	}
	port := os.Getenv("AGENT_PORT")
	if port == "" {
		if mode == "library" {
			port = "7003"
		} else {
			port = "7004"
		}
	}

	s := &server{
		mode: mode,
		port: port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /workload", s.handleWorkload)
	mux.HandleFunc("POST /stop", s.handleStop)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /info", s.handleInfo)
	mux.HandleFunc("/metrics", s.handleMetrics)

	log.Printf("Go benchmark agent starting mode=%s port=%s", mode, port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func (s *server) handleWorkload(w http.ResponseWriter, r *http.Request) {
	var req WorkloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		http.Error(w, "workload already running", http.StatusConflict)
		return
	}

	s.recorder = workloads.NewMetricsRecorder()

	runner := s.createRunner(req.Type, req.Config)
	if runner == nil {
		s.mu.Unlock()
		http.Error(w, fmt.Sprintf("unknown workload: %s", req.Type), http.StatusBadRequest)
		return
	}

	s.runner = runner
	s.runID = fmt.Sprintf("go-%s-%s-%d", s.mode, req.Type, time.Now().UnixMilli())
	s.workload = req.Type
	s.running = true

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	if req.Config.DBURL == "" {
		req.Config.DBURL = os.Getenv("DB_URL")
		if req.Config.DBURL == "" {
			req.Config.DBURL = "postgresql://root@localhost:26257/benchmark?sslmode=disable"
		}
	}

	if req.Config.WarmupSecs > 0 {
		s.recorder.SetWarmup(time.Duration(req.Config.WarmupSecs) * time.Second)
	}

	go func() {
		if err := runner.Start(ctx, req.Config); err != nil {
			log.Printf("workload error: %v", err)
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	go s.emitMetrics(ctx)

	json.NewEncoder(w).Encode(map[string]string{
		"run_id": s.runID,
		"status": "started",
	})
}

func (s *server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		json.NewEncoder(w).Encode(map[string]string{"status": "idle"})
		return
	}

	if s.cancel != nil {
		s.cancel()
	}
	if s.runner != nil {
		s.runner.Stop()
	}
	s.running = false

	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"language": "go",
		"mode":     s.mode,
	})
}

func (s *server) handleInfo(w http.ResponseWriter, r *http.Request) {
	driver := "pgx/v5"
	libVersion := "n/a"
	if s.mode == "library" {
		libVersion = "crdbgo-0.1.0"
	}
	json.NewEncoder(w).Encode(map[string]string{
		"language":        "go",
		"library_version": libVersion,
		"driver":          driver,
	})
}

func (s *server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ch := make(chan workloads.MetricSnapshot, 16)
	s.subMu.Lock()
	s.metricSub = append(s.metricSub, ch)
	s.subMu.Unlock()

	defer func() {
		s.subMu.Lock()
		for i, sub := range s.metricSub {
			if sub == ch {
				s.metricSub = append(s.metricSub[:i], s.metricSub[i+1:]...)
				break
			}
		}
		s.subMu.Unlock()
		close(ch)
	}()

	for snap := range ch {
		data, _ := json.Marshal(snap)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

func (s *server) emitMetrics(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.running || s.recorder == nil {
				s.mu.Unlock()
				return
			}
			snap := s.recorder.Snapshot(s.runID, s.mode, s.workload)
			s.mu.Unlock()

			s.subMu.Lock()
			for _, ch := range s.metricSub {
				select {
				case ch <- snap:
				default:
				}
			}
			s.subMu.Unlock()
		}
	}
}

func (s *server) createRunner(workloadType string, config workloads.WorkloadConfig) workloads.WorkloadRunner {
	if workloadType == "scenario" && len(config.ScenarioData) > 0 {
		return workloads.CreateScenarioRunner(s.recorder, config.ScenarioData, s.mode)
	}
	return workloads.CreateRunner(workloadType, s.mode == "library", s.recorder)
}
