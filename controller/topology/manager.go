package topology

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
)

type NodeInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Port   int    `json:"port"`
}

type Manager struct {
	mu         sync.RWMutex
	deployDir  string
	active     protocol.Topology
	running    bool
	remoteURL  string
	nodes      []NodeInfo
	composeBin string
}

func New(deployDir string) *Manager {
	bin := "docker-compose"
	if _, err := exec.LookPath("docker-compose"); err != nil {
		bin = "docker"
	}
	return &Manager{
		deployDir:  deployDir,
		composeBin: bin,
	}
}

func (m *Manager) composeExec(ctx context.Context, file string, args ...string) *exec.Cmd {
	if m.composeBin == "docker" {
		fullArgs := append([]string{"compose", "-f", file, "-p", "crdb-bench"}, args...)
		return exec.CommandContext(ctx, "docker", fullArgs...)
	}
	fullArgs := append([]string{"-f", file, "-p", "crdb-bench"}, args...)
	return exec.CommandContext(ctx, "docker-compose", fullArgs...)
}

func (m *Manager) DeployDir() string {
	return m.deployDir
}

func (m *Manager) SetRemoteURL(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remoteURL = url
}

func (m *Manager) MarkRunning(topo protocol.Topology) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = topo
	m.running = true
}

func (m *Manager) IsRemote() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.remoteURL != ""
}

func (m *Manager) DBURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.remoteURL != "" {
		return m.remoteURL
	}
	switch m.active {
	case protocol.TopoSingle:
		return "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
	case protocol.TopoCluster:
		return "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
	case protocol.TopoMulti:
		return "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
	default:
		return "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
	}
}

func (m *Manager) Active() protocol.Topology {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

func (m *Manager) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

func (m *Manager) composeFile(topo protocol.Topology) string {
	return fmt.Sprintf("docker-compose.%s.yml", topo)
}

func (m *Manager) Start(ctx context.Context, topo protocol.Topology) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.remoteURL != "" {
		m.active = topo
		m.running = true
		return nil
	}

	if m.running {
		m.active = topo
		return nil
	}

	file := m.composeFile(topo)
	cmd := m.composeExec(ctx, file, "up", "-d", "--wait")
	cmd.Dir = m.deployDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start topology %s: %w\n%s", topo, err, string(out))
	}

	m.active = topo
	m.running = true
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.remoteURL != "" {
		m.running = false
		m.active = ""
		return nil
	}

	file := m.composeFile(m.active)
	cmd := m.composeExec(ctx, file, "down", "-v")
	cmd.Dir = m.deployDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop topology: %w\n%s", err, string(out))
	}

	m.running = false
	m.active = ""
	m.nodes = nil
	return nil
}

func (m *Manager) Health(ctx context.Context) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return false, nil
	}

	if m.remoteURL != "" {
		return true, nil
	}

	file := m.composeFile(m.active)
	cmd := m.composeExec(ctx, file, "ps", "--format", "{{.Name}} {{.Status}}")
	cmd.Dir = m.deployDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("health check failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(line), "up") {
			return false, nil
		}
	}
	return true, nil
}

func (m *Manager) Nodes(ctx context.Context) ([]NodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("no topology running")
	}

	if m.remoteURL != "" {
		return []NodeInfo{{Name: "remote", Status: "up", Port: 26257}}, nil
	}

	file := m.composeFile(m.active)
	cmd := m.composeExec(ctx, file, "ps", "--format", "{{.Name}} {{.Status}} {{.Ports}}")
	cmd.Dir = m.deployDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list nodes failed: %w", err)
	}

	var nodes []NodeInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		nodes = append(nodes, NodeInfo{
			Name:   parts[0],
			Status: parts[1],
		})
	}
	return nodes, nil
}

func (m *Manager) ExecOnNode(ctx context.Context, containerName string, command []string) (string, error) {
	args := append([]string{"exec", containerName}, command...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (m *Manager) StopNode(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", containerName)
	_, err := cmd.CombinedOutput()
	return err
}

func (m *Manager) StartNode(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "start", containerName)
	_, err := cmd.CombinedOutput()
	return err
}

func (m *Manager) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("topology not ready after %v", timeout)
		case <-ticker.C:
			healthy, err := m.Health(ctx)
			if err == nil && healthy {
				return nil
			}
		}
	}
}

func (m *Manager) ContainerNames(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running || m.remoteURL != "" {
		return nil, nil
	}

	file := m.composeFile(m.active)
	cmd := m.composeExec(ctx, file, "ps", "--format", "{{.Name}}")
	cmd.Dir = m.deployDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}
