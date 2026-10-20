# Benchmark Workflow Plan

Step-by-step guide for running CockroachDB Developer Benchmarks locally with Docker.

## Prerequisites

Before starting, ensure you have:

- [ ] Docker Desktop running (with at least 8GB RAM allocated)
- [ ] Go 1.22+ installed (`go version`)
- [ ] Node.js 20+ installed (`node --version`)
- [ ] Ports available: 3000, 7001-7008, 8080, 9090, 26257

## Demo Flow Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Phase 1: Start CockroachDB (Docker)                               │
│  make topo-up TOPOLOGY=single|cluster|multi                        │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│  Phase 2: Build & Start Agents (Docker)                            │
│  make agents-up                                                     │
│  → 8 containers: Java/Go/Python/.NET × library/vanilla             │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│  Phase 3: Start Controller + Dashboard                             │
│  make dev                                                           │
│  → Controller :9090  │  Dashboard :3000                            │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│  Phase 4: Run Benchmarks                                           │
│  make bench-all  or  make bench WORKLOAD=crud                      │
│  → Watch live at http://localhost:3000                              │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│  Phase 5: Generate Report                                          │
│  make report                                                        │
│  → HTML report saved to results/report-<timestamp>.html            │
└─────────────────────────────────────────────────────────────────────┘
```

## Phase 1: Start CockroachDB

Choose one topology based on what you want to benchmark:

### Single Node (fast iteration, ~10s startup)

```bash
make topo-up
```

- Supports: CRUD, Retry Storm, Mixed R/W, Vector Search
- Does NOT support: Follower Reads (needs multiple nodes), Chaos Engineering
- CockroachDB UI: http://localhost:8080

### 3-Node Cluster (full workload suite, ~30s startup)

```bash
make topo-up TOPOLOGY=cluster
```

- Supports: All 6 workloads including Follower Reads
- Does NOT support: Chaos Engineering (needs 9 nodes)
- CockroachDB UI: http://localhost:8080

### 9-Node Multi-Region (chaos engineering, ~60s startup)

```bash
make topo-up TOPOLOGY=multi
```

- Supports: All 6 workloads + all 6 chaos scenarios
- Regions: US-EAST (nodes 1-3), US-WEST (nodes 4-6), EU-WEST (nodes 7-9)
- CockroachDB UI: http://localhost:8080
- Requires ~6GB RAM for CockroachDB alone

### Verify CockroachDB is ready

```bash
docker exec -it crdb-bench-crdb-1-1 cockroach sql --insecure -e "SELECT 1"
```

## Phase 2: Build & Start Agents

### All 8 agents (recommended for full comparison)

```bash
make agents-up
```

This builds Docker images and starts containers for:

| Container | Port | Language | Mode |
|-----------|------|----------|------|
| agent-java-lib | 7001 | Java 21 (Spring Boot) | Library |
| agent-java-vanilla | 7002 | Java 21 (JDBC) | Vanilla |
| agent-go-lib | 7003 | Go (crdbgo) | Library |
| agent-go-vanilla | 7004 | Go (pgx) | Vanilla |
| agent-python-lib | 7005 | Python (cockroachdb-py) | Library |
| agent-python-vanilla | 7006 | Python (psycopg3) | Vanilla |
| agent-dotnet-lib | 7007 | .NET 9 (CockroachDB.DotNet) | Library |
| agent-dotnet-vanilla | 7008 | .NET 9 (Npgsql) | Vanilla |

First build takes 5-10 minutes (downloading base images). Subsequent builds use Docker cache.

### Specific languages only (faster for development)

```bash
make agents-up AGENTS=go,python
```

### Verify agents are running

```bash
curl -s http://localhost:7001/health | python3 -m json.tool
curl -s http://localhost:7003/health | python3 -m json.tool
```

## Phase 3: Start Controller + Dashboard

### Development mode (hot reload)

```bash
make dev
```

- Controller: http://localhost:9090
- Dashboard: http://localhost:3000

### Production mode

```bash
make start
```

### Custom ports

```bash
make dev PORT=5679 DASHBOARD_PORT=3001
```

### With remote CockroachDB (skip Docker topology)

```bash
make dev DB_URL="postgresql://user:pass@remote-host:26257/benchmark?sslmode=verify-full"
```

## Phase 4: Run Benchmarks

### Option A: From the Dashboard UI

1. Open http://localhost:3000
2. Use the **Benchmark Controls** panel on the Live page
3. Select topology, workloads, duration, concurrency
4. Click **Start**
5. Watch charts update in real-time

### Option B: From the command line

```bash
# Single workload
make bench WORKLOAD=crud

# All 6 workloads sequentially (~6 minutes)
make bench-all

# Custom duration and concurrency
make bench WORKLOAD=retry_storm DURATION=120 CONCURRENCY=32

# Chaos engineering gauntlet (~9 minutes, requires 9-node topology)
make bench-chaos

# Random chaos mode
make bench-chaos CHAOS_MODE=random
```

### What to watch during benchmarks

Open the dashboard at http://localhost:3000 and observe:

1. **Agent Status Bar** — Shows all 8 agents with live ops/sec
2. **Workload Tabs** — Switch between CRUD, Retry Storm, Mixed R/W, etc.
3. **Metrics Chart** — All 8 runners on the same chart (solid = library, dashed = vanilla)
4. **Metric Selector** — Switch between ops/sec, p50/p95/p99 latency, errors, retries, CPU, memory
5. **Chaos Events** — Red vertical lines on the chart when chaos is injected (9-node only)

## Phase 5: Generate HTML Report

### Download the latest run's report

```bash
make report
```

Report saved to `results/report-<timestamp>.html`. Open in any browser.

### Download a specific run's report

```bash
curl -o my-report.html http://localhost:9090/api/runs/<run-id>/report
```

### What the report contains

- **Summary table** — All 8 agents with avg ops/sec, p50/p95/p99 latency, errors, retries
- **Per-workload charts** — ops/sec over time + latency bar charts for each of the 6 workloads
- **Library vs Vanilla comparison** — Bar chart showing overhead percentage per language
- **Chaos timeline** — When chaos events occurred and their impact (if applicable)

## Recommended Demo Sequence

For presentations or evaluations, run these in order:

| Step | Command | Topology | Duration | What it proves |
|------|---------|----------|----------|----------------|
| 1 | `make topo-up` | Single | — | Start CockroachDB |
| 2 | `make agents-up` | — | — | Start all agents |
| 3 | `make dev` | — | — | Start dashboard |
| 4 | `make bench WORKLOAD=crud` | Single | 60s | Basic overhead — how much does the library cost? |
| 5 | `make bench WORKLOAD=retry_storm` | Single | 60s | **The money shot** — auto-retry vs hand-rolled retry |
| 6 | `make report` | — | — | Save CRUD + retry storm results |
| 7 | `make topo-down && make topo-up TOPOLOGY=cluster` | 3-node | — | Scale up topology |
| 8 | `make bench WORKLOAD=follower_reads` | Cluster | 60s | Library routing reads to nearest replica |
| 9 | `make bench-all` | Cluster | ~6 min | Complete 6-workload comparison |
| 10 | `make report` | — | — | Save full suite results |
| 11 | `make topo-down && make topo-up TOPOLOGY=multi` | 9-node | — | Scale to multi-region |
| 12 | `make bench-chaos` | Multi | ~9 min | Resilience under node failures and partitions |
| 13 | `make report` | — | — | Save chaos results — the most impactful report |

### Key talking points at each step

**Step 4 (CRUD):** "The library adds connection pool optimization and middleware pipeline. Expect 2-8% overhead on basic operations — the cost of abstraction."

**Step 5 (Retry Storm):** "Under high contention, vanilla code with hand-rolled retry shows 3-5x more failed transactions. The library's exponential backoff + jitter (Python uses server-side savepoint restart) spreads retries intelligently."

**Step 8 (Follower Reads):** "Libraries route reads to the nearest replica automatically. Expect 30-60% lower read latency on multi-node clusters without any SQL changes."

**Step 12 (Chaos):** "When nodes fail, libraries with circuit breakers recover gracefully. Vanilla code hard-fails until manual intervention. This is where the library pays for itself."

## Cleanup

```bash
make stop          # Stop controller + dashboard
make agents-down   # Stop agent containers
make topo-down     # Stop and remove CockroachDB
make clean         # Full cleanup (removes results, containers, volumes)
```

## Troubleshooting

### Docker build fails

```bash
# Rebuild without cache
docker compose -f deploy/docker-compose.agents.yml build --no-cache
```

### Agent can't connect to CockroachDB

```bash
# Check CockroachDB is ready
docker exec -it crdb-bench-crdb-1-1 cockroach sql --insecure -e "SHOW DATABASES"

# Check network
docker network ls | grep bench
```

### Port already in use

```bash
# Use custom ports
make dev PORT=9091 DASHBOARD_PORT=3001
```

### Out of memory

The 9-node topology + 8 agents requires ~10GB RAM. If Docker is constrained:
- Start with single-node topology
- Run only 2 languages: `make agents-up AGENTS=go,python`
- Increase Docker Desktop memory allocation in Settings → Resources
