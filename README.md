# CockroachDB Developer Benchmarks

Benchmark framework for comparing 4 CockroachDB client libraries against their vanilla (raw driver) counterparts. Measures performance overhead, resilience benefits, and behavior under chaos conditions.

> **New here?** Start with the [Workflow Plan](WORKFLOW_PLAN.md) — a step-by-step guide for running benchmarks locally with Docker, including a recommended demo sequence with talking points.
>
> See [STATUS.md](STATUS.md) for current project status, known issues, and next steps.

## Libraries Under Test

| Library | Language | Vanilla Baseline |
|---------|----------|-----------------|
| `cockroachdb-spring-boot-starter` | Java / Spring Boot | JDBC + JdbcTemplate |
| `cockroachdb-go-library` (crdbgo) | Go | pgx/v5 + cockroach-go |
| `cockroachdb-python-library` | Python | psycopg3 |
| `cockroachdb-dotnet-library` | C# / .NET 9 | Npgsql |

## Architecture

```
┌─────────────────────────┐
│    React Dashboard      │  ← Live charts, history, comparison
│    (Vite + uPlot)       │
└────────┬────────────────┘
         │ ws:// + http://
┌────────▼────────────────┐
│    Go Controller        │  ← Orchestrator (:9090)
│  ┌──────┬──────┬──────┐ │
│  │Topo  │Sched │Chaos │ │
│  │Mgr   │uler  │Eng.  │ │
│  └──────┴──────┴──────┘ │
│  ┌──────────────────┐   │
│  │ Metrics Collector │   │
│  │ (SQLite + JSON)   │   │
│  └──────────────────┘   │
└────────┬────────────────┘
         │ http + ws://
┌────────▼────────────────────────────────────────────┐
│  Agents (8 containers: 4 languages × 2 modes)      │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌──────────┐ │
│  │Java     │ │Go       │ │Python   │ │.NET      │ │
│  │lib:7001 │ │lib:7003 │ │lib:7005 │ │lib:7007  │ │
│  │raw:7002 │ │raw:7004 │ │raw:7006 │ │raw:7008  │ │
│  └─────────┘ └─────────┘ └─────────┘ └──────────┘ │
└────────┬────────────────────────────────────────────┘
         │ pg wire protocol
┌────────▼────────────────┐
│    CockroachDB          │
│  single | 3-node | 9-node│
└─────────────────────────┘
```

## Quick Start

> For the full walkthrough with demo sequence and talking points, see [WORKFLOW_PLAN.md](WORKFLOW_PLAN.md).

### Prerequisites

- Docker and Docker Compose
- Go 1.22+
- Node.js 20+ (for dashboard)
- (Optional) JDK 21, Python 3.12+, .NET 9 SDK for local agent development

### 1. Start CockroachDB

```bash
# Single node (fast iteration)
make topo-up

# 3-node cluster (full workload suite)
make topo-up TOPOLOGY=cluster

# 9-node multi-region (chaos engineering)
make topo-up TOPOLOGY=multi
```

### 2. Start the Benchmark System

```bash
# Build and start controller + dashboard
make start

# Custom ports
make start CONTROLLER_PORT=9090 DASHBOARD_PORT=3001

# Development mode (hot reload)
make dev
```

### 3. Start Agents

```bash
# All 8 agents (4 languages × 2 modes)
make agents-up

# Specific languages only
make agents-up AGENTS=go,python
```

### 4. Run Benchmarks

```bash
# Single workload
make bench WORKLOAD=crud

# All 6 workloads
make bench-all

# Chaos engineering (requires 9-node topology)
make bench-chaos
```

### 5. View Results

Open the dashboard at **http://localhost:3000**

- **Live** — Real-time charts during benchmark execution
- **SQL** — Library vs Vanilla code comparison per language
- **History** — Browse past benchmark runs
- **Compare** — Side-by-side comparison of two runs
- **Monitoring** — Prometheus & Grafana integration

### 6. Prometheus & Grafana (Optional)

```bash
# Start monitoring stack
make monitoring-up

# Open Grafana (auto-provisioned dashboard)
open http://localhost:3001/d/crdb-benchmarks
# Login: admin / benchmark
```

Pre-built Grafana dashboard with throughput, latency, errors, retries, CPU, and memory — color-coded per language, auto-updated every 5 seconds.

### 7. CI Benchmarks (GitHub Actions)

Run benchmarks manually from GitHub Actions — results uploaded as downloadable artifacts (HTML report + JSON).

1. Go to **Actions** → **Run Benchmark** → **Run workflow**
2. Select topology, workloads, duration, concurrency
3. Download the HTML report from the artifacts when complete

No automatic triggers — runs only when you manually dispatch it.

### Remote CockroachDB

Skip Docker and benchmark against a remote cluster:

```bash
make start DB_URL="postgresql://user:pass@remote-host:26257/benchmark?sslmode=verify-full"
```

## Workloads

| # | Workload | What it Measures | Key Metric |
|---|----------|-----------------|------------|
| 1 | **CRUD** | Basic INSERT/SELECT/UPDATE/DELETE throughput | ops/sec, latency percentiles |
| 2 | **Retry Storm** | High-contention writes forcing serialization conflicts | retry success rate, p99 latency |
| 3 | **Mixed R/W** | 80/20 read/write realistic mix | throughput under mixed load |
| 4 | **Follower Reads** | `AS OF SYSTEM TIME` read throughput | read latency reduction |
| 5 | **CDC** | Changefeed event processing rate | event lag (ms) |
| 6 | **Vector Search** | pgvector similarity queries | search latency, indexing speed |

## Chaos Engineering

Available on 9-node multi-region topology (3 regions × 3 nodes):

| Scenario | What Happens | Library Advantage |
|----------|-------------|-------------------|
| **Node Kill** | Stop a random node for 30s | Auto-retry + circuit breaker vs manual reconnect |
| **Network Partition** | Isolate one region for 15s | Region-aware routing |
| **Region Failure** | Kill all 3 nodes in a region | Multi-region fallback |
| **Latency Injection** | Add 100ms latency to a region | Latency-aware pool management |
| **Rolling Restart** | Restart nodes one-by-one | Graceful retry + backoff |
| **Disk I/O Pressure** | Throttle disk on nodes | Circuit breaker activation |

Chaos modes:
- `individual` — One scenario per benchmark
- `gauntlet` — All 6 sequentially (~9 min)
- `random` — Random events during long-running soak tests

## Metrics Collected

- **Latency**: p50, p95, p99, max (microseconds)
- **Throughput**: ops/sec, total operations
- **Resources**: CPU %, memory RSS, active connections
- **Errors**: error count, retry count, retry success rate, circuit breaker state

## What to Observe

### Library vs Vanilla Comparison

The benchmarks reveal the real-world cost and benefit of using the CockroachDB client libraries:

1. **CRUD Overhead** — How much latency does the library's middleware pipeline add to basic operations? Expect 2-8% overhead for connection pool optimization and error classification.

2. **Retry Storm Value** — This is where libraries shine. Vanilla code with hand-rolled retry loops typically shows 3-5x more failed transactions under high contention. The library's exponential backoff + jitter spreads retries intelligently.

3. **Follower Read Routing** — Libraries that automatically route reads to followers show 30-60% lower read latency on multi-node clusters. Vanilla code requires manual SQL modification.

4. **Chaos Resilience** — Under node failures and network partitions, libraries with circuit breakers and connection pool recovery handle degraded conditions gracefully. Vanilla code often hard-fails until manual intervention.

5. **CDC Throughput** — Library changefeed handlers provide typed event parsing with minimal overhead. Compare against raw JSON parsing + manual HTTP endpoint code.

6. **Vector Search Abstraction** — VectorSearchTemplate adds negligible overhead (<1ms) while providing a clean API vs raw SQL.

### Cross-Language Comparison

- **Go** typically shows lowest raw latency due to goroutine-based concurrency
- **Java** shows strong throughput with virtual threads (Project Loom)
- **Python** has higher per-operation latency but competitive throughput with connection pooling
- **.NET** falls between Java and Go for most workloads

## Project Structure

```
cockroachdb-developer-benchmarks/
├── controller/          # Go backend (orchestrator)
│   ├── cmd/server/      # Entry point
│   ├── topology/        # Docker orchestration
│   ├── scheduler/       # Workload dispatch
│   ├── chaos/           # Failure injection
│   ├── collector/       # Metrics + storage
│   ├── api/             # HTTP + WebSocket
│   └── protocol/        # Shared types
├── agents/
│   ├── java/            # Spring Boot agent
│   ├── go/              # Go agent
│   ├── python/          # FastAPI agent
│   └── dotnet/          # .NET minimal API agent
├── dashboard/           # React + Vite + uPlot
├── deploy/              # Docker Compose files
│   ├── docker-compose.single.yml
│   ├── docker-compose.cluster.yml
│   ├── docker-compose.multi.yml
│   └── docker-compose.agents.yml
├── results/             # JSON benchmark exports
├── Makefile             # All commands
└── benchmark.db         # SQLite historical data
```

## Makefile Reference

```bash
# Lifecycle
make start                              # Start controller + dashboard
make stop                               # Stop all
make restart                            # Stop + start
make dev                                # Dev mode (hot reload)

# Topology
make topo-up                            # Start CockroachDB (default: single)
make topo-up TOPOLOGY=cluster           # 3-node cluster
make topo-up TOPOLOGY=multi             # 9-node multi-region
make topo-down                          # Tear down CockroachDB

# Agents
make agents-up                          # All 8 agent containers
make agents-up AGENTS=go,python         # Specific languages
make agents-down                        # Stop agents
make agents-build                       # Rebuild images

# Benchmarks
make bench WORKLOAD=crud                # Single workload
make bench-all                          # All 6 workloads
make bench-chaos                        # Chaos gauntlet (9-node)
make bench-chaos CHAOS_MODE=random      # Random chaos events

# Maintenance
make test                               # Run agent tests
make clean                              # Full cleanup
make help                               # Show all commands
```

## Configuration

All Makefile variables can be overridden:

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROLLER_PORT` | 9090 | Go controller HTTP port |
| `DASHBOARD_PORT` | 3000 | React dashboard port |
| `TOPOLOGY` | single | CockroachDB topology (single/cluster/multi) |
| `DB_URL` | (empty) | Remote CockroachDB URL (skips Docker) |
| `WORKLOAD` | crud | Workload to run with `make bench` |
| `DURATION` | 60 | Benchmark duration in seconds |
| `CONCURRENCY` | 16 | Number of concurrent workers |
| `AGENTS` | java,go,python,dotnet | Languages to include |
| `CHAOS_MODE` | gauntlet | Chaos mode (individual/gauntlet/random) |
