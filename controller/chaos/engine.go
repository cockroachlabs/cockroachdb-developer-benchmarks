package chaos

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/topology"
)

var allScenarios = []protocol.ChaosScenario{
	protocol.ChaosNodeKill,
	protocol.ChaosNetworkPartition,
	protocol.ChaosRegionFailure,
	protocol.ChaosLatencyInjection,
	protocol.ChaosRollingRestart,
	protocol.ChaosDiskPressure,
}

type EventCallback func(protocol.ChaosEvent)

type Engine struct {
	mu       sync.RWMutex
	topo     *topology.Manager
	events   []protocol.ChaosEvent
	running  bool
	cancel   context.CancelFunc
	callback EventCallback
}

func New(topo *topology.Manager) *Engine {
	return &Engine{
		topo: topo,
	}
}

func (e *Engine) SetCallback(cb EventCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callback = cb
}

func (e *Engine) Events() []protocol.ChaosEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	copied := make([]protocol.ChaosEvent, len(e.events))
	copy(copied, e.events)
	return copied
}

func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

func (e *Engine) Start(ctx context.Context, mode protocol.ChaosMode, scenarios []protocol.ChaosScenario) error {
	if e.topo.Active() != protocol.TopoMulti {
		return fmt.Errorf("chaos engine only active on multi-region topology")
	}

	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("chaos engine already running")
	}

	chaosCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.running = true
	e.events = nil
	e.mu.Unlock()

	if len(scenarios) == 0 {
		scenarios = allScenarios
	}

	go e.run(chaosCtx, mode, scenarios)
	return nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
	}
	e.running = false
}

func (e *Engine) TriggerSingle(ctx context.Context, scenario protocol.ChaosScenario, target string) error {
	if e.topo.Active() != protocol.TopoMulti {
		return fmt.Errorf("chaos engine only active on multi-region topology")
	}

	containers, err := e.topo.ContainerNames(ctx)
	if err != nil || len(containers) == 0 {
		return fmt.Errorf("cannot get containers: %v", err)
	}

	if target == "" {
		target = containers[rand.Intn(len(containers))]
	}

	go func() {
		switch scenario {
		case protocol.ChaosNodeKill:
			e.nodeKill(ctx, target)
		case protocol.ChaosNetworkPartition:
			e.networkPartition(ctx, target)
		case protocol.ChaosRegionFailure:
			e.regionFailure(ctx, containers)
		case protocol.ChaosLatencyInjection:
			e.latencyInjection(ctx, target)
		case protocol.ChaosRollingRestart:
			e.rollingRestart(ctx, containers)
		case protocol.ChaosDiskPressure:
			e.diskPressure(ctx, target)
		default:
			log.Printf("chaos: unknown scenario %s", scenario)
		}
	}()

	return nil
}

func (e *Engine) run(ctx context.Context, mode protocol.ChaosMode, scenarios []protocol.ChaosScenario) {
	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	switch mode {
	case protocol.ChaosModeIndividual:
		e.runIndividual(ctx, scenarios)
	case protocol.ChaosModeGauntlet:
		e.runGauntlet(ctx, scenarios)
	case protocol.ChaosModeRandom:
		e.runRandom(ctx, scenarios)
	}
}

func (e *Engine) runIndividual(ctx context.Context, scenarios []protocol.ChaosScenario) {
	if len(scenarios) > 0 {
		e.executeScenario(ctx, scenarios[0])
	}
}

func (e *Engine) runGauntlet(ctx context.Context, scenarios []protocol.ChaosScenario) {
	for _, s := range scenarios {
		select {
		case <-ctx.Done():
			return
		default:
		}
		e.executeScenario(ctx, s)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (e *Engine) runRandom(ctx context.Context, scenarios []protocol.ChaosScenario) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(5+rand.Intn(15)) * time.Second):
			idx := rand.Intn(len(scenarios))
			e.executeScenario(ctx, scenarios[idx])
		}
	}
}

func (e *Engine) executeScenario(ctx context.Context, scenario protocol.ChaosScenario) {
	containers, err := e.topo.ContainerNames(ctx)
	if err != nil || len(containers) == 0 {
		log.Printf("chaos: cannot get containers: %v", err)
		return
	}

	target := containers[rand.Intn(len(containers))]

	switch scenario {
	case protocol.ChaosNodeKill:
		e.nodeKill(ctx, target)
	case protocol.ChaosNetworkPartition:
		e.networkPartition(ctx, target)
	case protocol.ChaosRegionFailure:
		e.regionFailure(ctx, containers)
	case protocol.ChaosLatencyInjection:
		e.latencyInjection(ctx, target)
	case protocol.ChaosRollingRestart:
		e.rollingRestart(ctx, containers)
	case protocol.ChaosDiskPressure:
		e.diskPressure(ctx, target)
	}
}

func (e *Engine) recordEvent(scenario protocol.ChaosScenario, action, target string, duration time.Duration) {
	ev := protocol.ChaosEvent{
		Timestamp: time.Now(),
		Scenario:  scenario,
		Action:    action,
		Target:    target,
		Duration:  duration,
	}
	e.mu.Lock()
	e.events = append(e.events, ev)
	cb := e.callback
	e.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
	log.Printf("chaos: %s %s on %s", scenario, action, target)
}

func (e *Engine) nodeKill(ctx context.Context, target string) {
	e.recordEvent(protocol.ChaosNodeKill, "stop", target, 0)
	if err := e.topo.StopNode(ctx, target); err != nil {
		log.Printf("chaos: node_kill stop failed: %v", err)
		return
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
	}

	e.recordEvent(protocol.ChaosNodeKill, "start", target, 15*time.Second)
	if err := e.topo.StartNode(ctx, target); err != nil {
		log.Printf("chaos: node_kill restart failed: %v", err)
	}
}

func (e *Engine) networkPartition(ctx context.Context, target string) {
	e.recordEvent(protocol.ChaosNetworkPartition, "isolate", target, 0)
	_, _ = e.topo.ExecOnNode(ctx, target, []string{"iptables", "-A", "INPUT", "-p", "tcp", "--dport", "26257", "-j", "DROP"})

	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
	}

	e.recordEvent(protocol.ChaosNetworkPartition, "restore", target, 10*time.Second)
	_, _ = e.topo.ExecOnNode(ctx, target, []string{"iptables", "-D", "INPUT", "-p", "tcp", "--dport", "26257", "-j", "DROP"})
}

func (e *Engine) regionFailure(ctx context.Context, containers []string) {
	regionContainers := filterByPrefix(containers, "roach-us-east")
	if len(regionContainers) == 0 && len(containers) > 0 {
		regionContainers = containers[:1]
	}

	for _, c := range regionContainers {
		e.recordEvent(protocol.ChaosRegionFailure, "stop", c, 0)
		_ = e.topo.StopNode(ctx, c)
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Second):
	}

	for _, c := range regionContainers {
		e.recordEvent(protocol.ChaosRegionFailure, "start", c, 20*time.Second)
		_ = e.topo.StartNode(ctx, c)
	}
}

func (e *Engine) latencyInjection(ctx context.Context, target string) {
	e.recordEvent(protocol.ChaosLatencyInjection, "inject", target, 0)
	_, _ = e.topo.ExecOnNode(ctx, target, []string{"tc", "qdisc", "add", "dev", "eth0", "root", "netem", "delay", "200ms", "50ms"})

	select {
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
	}

	e.recordEvent(protocol.ChaosLatencyInjection, "remove", target, 15*time.Second)
	_, _ = e.topo.ExecOnNode(ctx, target, []string{"tc", "qdisc", "del", "dev", "eth0", "root"})
}

func (e *Engine) rollingRestart(ctx context.Context, containers []string) {
	for _, c := range containers {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e.recordEvent(protocol.ChaosRollingRestart, "restart", c, 0)
		_ = e.topo.StopNode(ctx, c)
		time.Sleep(5 * time.Second)
		_ = e.topo.StartNode(ctx, c)
		time.Sleep(10 * time.Second)
	}
}

func (e *Engine) diskPressure(ctx context.Context, target string) {
	e.recordEvent(protocol.ChaosDiskPressure, "fill", target, 0)
	_, _ = e.topo.ExecOnNode(ctx, target, []string{"dd", "if=/dev/zero", "of=/tmp/fill", "bs=1M", "count=512"})

	select {
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
	}

	e.recordEvent(protocol.ChaosDiskPressure, "cleanup", target, 15*time.Second)
	_, _ = e.topo.ExecOnNode(ctx, target, []string{"rm", "-f", "/tmp/fill"})
}

func filterByPrefix(items []string, prefix string) []string {
	var result []string
	for _, item := range items {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			result = append(result, item)
		}
	}
	return result
}
