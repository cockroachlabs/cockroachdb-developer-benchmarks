package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/collector"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
	"github.com/gorilla/websocket"
)

type AgentStatus struct {
	Agent  protocol.AgentEntry `json:"agent"`
	RunID  string              `json:"run_id,omitempty"`
	Status string              `json:"status"`
	Error  string              `json:"error,omitempty"`
}

type RunState struct {
	ID        string
	Cancel    context.CancelFunc
	StartedAt time.Time
	Agents    []AgentStatus
}

type Scheduler struct {
	mu        sync.RWMutex
	agents    []protocol.AgentEntry
	collector *collector.Collector
	current   *RunState
	client    *http.Client
}

func New(agents []protocol.AgentEntry, coll *collector.Collector) *Scheduler {
	return &Scheduler{
		agents:    agents,
		collector: coll,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Scheduler) CurrentRun() *RunState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current != nil
}

func (s *Scheduler) StartRun(ctx context.Context, run protocol.BenchmarkRun) error {
	s.mu.Lock()
	if s.current != nil {
		s.mu.Unlock()
		return fmt.Errorf("benchmark %s already running", s.current.ID)
	}

	runCtx, cancel := context.WithCancel(ctx)
	state := &RunState{
		ID:        run.ID,
		Cancel:    cancel,
		StartedAt: time.Now(),
	}
	s.current = state
	s.mu.Unlock()

	go s.executeRun(runCtx, run, state)
	return nil
}

func (s *Scheduler) StopRun() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil {
		return fmt.Errorf("no benchmark running")
	}

	s.current.Cancel()
	s.current = nil
	return nil
}

func (s *Scheduler) StopAllAgents(ctx context.Context) {
	s.mu.RLock()
	agents := s.agents
	s.mu.RUnlock()

	s.stopAgents(ctx, agents)
}

func (s *Scheduler) executeRun(ctx context.Context, run protocol.BenchmarkRun, state *RunState) {
	defer func() {
		s.mu.Lock()
		if s.current == state {
			s.current = nil
		}
		s.mu.Unlock()
	}()

	agents := run.Agents
	if len(agents) == 0 {
		agents = s.agents
	}

	iterations := run.Iterations
	if iterations <= 0 {
		iterations = 1
	}

	for _, workload := range run.Workloads {
		for iter := 1; iter <= iterations; iter++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			iterRunID := run.ID
			if iterations > 1 {
				iterRunID = fmt.Sprintf("%s-iter-%d", run.ID, iter)
				log.Printf("Iteration %d/%d — workload %s", iter, iterations, workload)
			}

			iterRun := run
			iterRun.ID = iterRunID

			if run.Isolation {
				langGroups := groupAgentsByLanguage(agents)
				for lang, group := range langGroups {
					select {
					case <-ctx.Done():
						return
					default:
					}

					log.Printf("Running %s (isolated) — workload %s for run %s", lang, workload, iterRun.ID)
					s.dispatchWorkload(ctx, iterRun, workload, group)

					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Duration(run.Duration) * time.Second):
					}

					s.stopAgents(ctx, group)
					time.Sleep(2 * time.Second)
				}
			} else {
				log.Printf("starting workload %s for run %s", workload, iterRun.ID)
				s.dispatchWorkload(ctx, iterRun, workload, agents)

				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(run.Duration) * time.Second):
				}

				s.stopAgents(ctx, agents)
				if iter < iterations || workload != run.Workloads[len(run.Workloads)-1] {
					time.Sleep(2 * time.Second)
				}
			}
		}
	}

	log.Printf("run %s completed", run.ID)
}

func groupAgentsByLanguage(agents []protocol.AgentEntry) map[string][]protocol.AgentEntry {
	groups := make(map[string][]protocol.AgentEntry)
	for _, a := range agents {
		key := string(a.Language)
		groups[key] = append(groups[key], a)
	}
	return groups
}

func (s *Scheduler) dispatchWorkload(ctx context.Context, run protocol.BenchmarkRun, workload protocol.WorkloadType, agents []protocol.AgentEntry) {
	var wg sync.WaitGroup

	for _, agent := range agents {
		wg.Add(1)
		go func(a protocol.AgentEntry) {
			defer wg.Done()

			concurrency := run.Concurrency
			if concurrency <= 0 {
				concurrency = 8
			}
			req := protocol.WorkloadRequest{
				Type: workload,
				Mode: a.Mode,
				Config: protocol.WorkloadConfig{
					DurationSecs: run.Duration,
					Concurrency:  concurrency,
					WarmupSecs:   run.WarmupSecs,
					ScenarioData: run.ScenarioJSON,
				},
			}

			err := s.sendWorkload(ctx, a, req)
			if err != nil {
				log.Printf("failed to dispatch to %s/%s: %v", a.Language, a.Mode, err)
				return
			}

			go s.streamMetrics(ctx, a, run.ID)
		}(agent)
	}

	wg.Wait()
}

func (s *Scheduler) sendWorkload(ctx context.Context, agent protocol.AgentEntry, req protocol.WorkloadRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s:%d/workload", agent.Host, agent.Port)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("agent returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Scheduler) streamMetrics(ctx context.Context, agent protocol.AgentEntry, runID string) {
	url := fmt.Sprintf("ws://%s:%d/metrics", agent.Host, agent.Port)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		log.Printf("websocket connect failed for %s/%s: %v", agent.Language, agent.Mode, err)
		return
	}
	defer conn.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var snapshot protocol.MetricSnapshot
		err := conn.ReadJSON(&snapshot)
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Printf("metric stream error from %s/%s: %v", agent.Language, agent.Mode, err)
			}
			return
		}

		snapshot.RunID = runID
		s.collector.Ingest(snapshot)
	}
}

func (s *Scheduler) stopAgents(ctx context.Context, agents []protocol.AgentEntry) {
	var wg sync.WaitGroup
	for _, agent := range agents {
		wg.Add(1)
		go func(a protocol.AgentEntry) {
			defer wg.Done()
			url := fmt.Sprintf("http://%s:%d/stop", a.Host, a.Port)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
			if err != nil {
				return
			}
			resp, err := s.client.Do(req)
			if err != nil {
				log.Printf("failed to stop %s/%s: %v", a.Language, a.Mode, err)
				return
			}
			resp.Body.Close()
		}(agent)
	}
	wg.Wait()
}

func (s *Scheduler) HealthCheck(ctx context.Context, agent protocol.AgentEntry) (*protocol.HealthResponse, error) {
	url := fmt.Sprintf("http://%s:%d/health", agent.Host, agent.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var health protocol.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, err
	}
	return &health, nil
}

func (s *Scheduler) SetDBURL(dbURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx := range s.agents {
		s.agents[idx].Host = dbURL
	}
}
