# CockroachDB Developer Benchmarks — Design Spec

**Date:** 2026-06-09
**Status:** Approved
**Project:** cockroachdb-developer-benchmarks

## Purpose

Benchmark 4 CockroachDB client libraries against their vanilla (raw driver) counterparts to measure the performance overhead and resilience benefits of each abstraction layer. The system provides a live dashboard, supports local Docker clusters and remote connections, and includes chaos engineering for multi-region scenarios.

### Libraries Under Test

| Library | Language | Vanilla Baseline |
|---------|----------|-----------------|
| cockroachdb-spring-boot-starter | Java (Spring Boot) | Plain JDBC / JdbcTemplate, manual retry loops |
| cockroachdb-go-library (crdbgo) | Go | Raw pgx/v5 pool, `crdb.ExecuteTx()` from cockroach-go/v2 |
| cockroachdb-python-library | Python | Raw psycopg3 / asyncpg, manual retry loops |
| cockroachdb-dotnet-library | C# / .NET 9+ | Raw Npgsql + EF Core, manual Polly retry |

## Architecture: Controller + Agent

### System Layers

1. **React Dashboard** (Vite, uPlot) — Workload Focus View with tabs per workload type. All 8 runners displayed on the same chart. WebSocket for live metrics, HTTP for config/history.

2. **Go Controller** (:9090) — Four internal modules:
   - **Topology Manager** — Docker orchestration for 3 CockroachDB topologies
   - **Benchmark Scheduler** — Workload dispatch and agent coordination
   - **Chaos Engine** — 6 failure injection modes for multi-region scenarios
   - **Metrics Collector** — Receives agent streams, writes to SQLite + JSON

3. **Agents** (one per language, 8 containers total) — Each agent runs one mode (library or vanilla) and exposes a standard HTTP + WebSocket protocol. The controller treats all agents identically.

4. **CockroachDB** (Docker) — Three topology profiles selectable at launch.

### Communication Flow

```
Dashboard ←── ws:// + http:// ──→ Controller ←── http + ws:// ──→ Agents ←── pg wire ──→ CockroachDB
```

## CockroachDB Topologies

| Profile | Nodes | Regions | Use Case |
|---------|-------|---------|----------|
| `single` | 1 | — | Fast iteration. CRUD, retry storm, mixed R/W, vector search. |
| `cluster` | 3 | 1 | Full suite including follower reads and CDC. |
| `multi` | 9 | 3 (US-EAST, US-WEST, EU-WEST, 3 nodes each) | All workloads + chaos engineering. |

Topology selection via `make topo-up TOPOLOGY=single|cluster|multi`. For remote CockroachDB, pass `DB_URL` instead.

## Workloads

### 6 Benchmark Workloads

| # | Workload | Schema | Operations | Default Duration | Default Concurrency |
|---|----------|--------|------------|-----------------|-------------------|
| 1 | CRUD | `bench_accounts(id, balance, name, region)` | INSERT, SELECT by PK, UPDATE balance, DELETE | 60s | 16 workers |
| 2 | Retry Storm | Same table | Concurrent UPDATE on same rows to force SQLSTATE 40001 | 60s | 32 workers on 10 hot rows |
| 3 | Mixed R/W | accounts + `bench_orders(id, account_id, amount, ts)` | 80% reads / 20% writes | 60s | 16 workers |
| 4 | Follower Reads | Same tables | `AS OF SYSTEM TIME follower_read_timestamp()` vs current | 60s | 16 workers |
| 5 | CDC | Same tables | Continuous writes + changefeed consumer measuring event lag | 60s | 8 writers + 1 consumer |
| 6 | Vector Search | `bench_documents(id, content, embedding VECTOR(1536), metadata JSONB)` | INSERT embeddings + cosine similarity queries | 60s | 8 workers |

All durations and concurrency levels are configurable via the workload JSON sent from the controller.

### Library vs Vanilla Delta

| Workload | Library | Vanilla |
|----------|---------|---------|
| CRUD | Connection pool with CRDB defaults, middleware pipeline | Raw pool config, manual connection management |
| Retry Storm | Auto-retry with backoff + jitter (annotations/decorators/middleware) | Manual `BEGIN; ... COMMIT;` loop with SQLSTATE 40001 check |
| Mixed R/W | Template/session management, read/write routing | Manual transaction scoping |
| Follower Reads | `@FollowerRead` / decorator / middleware injects AS OF SYSTEM TIME | Manual SQL modification |
| CDC | Library webhook handler parses typed events | Manual HTTP endpoint + raw JSON parsing |
| Vector Search | VectorSearchTemplate / decorators handle embedding + search | Manual `SELECT ... ORDER BY embedding <=> $1 LIMIT $2` |

## Chaos Engineering

Runs only on the 9-node multi-region topology. Failures are injected *during* active benchmarks.

### 9-Node Layout

```
Region US-EAST (crdb-1, crdb-2, crdb-3)
Region US-WEST (crdb-4, crdb-5, crdb-6)
Region EU-WEST (crdb-7, crdb-8, crdb-9)
```

### 6 Chaos Scenarios

| Scenario | Docker Mechanism | Timing | Measurement |
|----------|-----------------|--------|-------------|
| Node Kill | `docker stop <node>` + restart after 30s | At 20s into workload | Failover latency, retry success, pool recovery |
| Network Partition | Docker network disconnect on target region | 15s duration mid-workload | Leaseholder migration, routing, error rate |
| Region Failure | `docker stop` all 3 nodes in one region | 30s duration | Quorum with 6/9 nodes, cross-region latency |
| Latency Injection | `tc netem delay 100ms 20ms` via Docker exec | Throughout workload | Follower read routing, latency-aware pool |
| Rolling Restart | Sequential `docker restart` with 10s gaps | 3 restarts over 30s | Zero-downtime continuity, connection draining |
| Disk I/O Pressure | cgroups I/O throttle via Docker `--device-write-bps` | 20s duration | Write latency, circuit breaker activation |

### Chaos Run Modes

- **Individual** — One scenario per benchmark for isolated measurement
- **Gauntlet** — All 6 sequentially in one extended run (~9 min)
- **Random** — Random events during long-running soak tests

Chaos events are timestamped in the metrics stream for dashboard overlay.

## Agent Protocol

### HTTP Endpoints

```
POST /workload
  Body: {
    "type": "crud|retry_storm|mixed_rw|follower_reads|cdc|vector_search",
    "mode": "library|vanilla",
    "config": {
      "duration_secs": 60,
      "concurrency": 16,
      "db_url": "postgresql://...",
      "hot_rows": 10
    }
  }
  Response: { "run_id": "uuid", "status": "running" }

POST /stop
  Response: { "status": "stopped" }

GET /health
  Response: { "status": "ok", "language": "go", "mode": "library" }

GET /info
  Response: { "language": "go", "library_version": "0.1.0", "driver": "pgx/v5" }
```

### WebSocket Metrics Stream (`ws:///metrics`)

Streams a snapshot every second during a workload run:

```json
{
  "ts": 1706000101,
  "run_id": "uuid",
  "language": "go",
  "mode": "library",
  "workload": "retry_storm",
  "ops_total": 14520,
  "ops_per_sec": 1245,
  "latency_p50_us": 820,
  "latency_p95_us": 2340,
  "latency_p99_us": 8100,
  "latency_max_us": 15200,
  "errors": 3,
  "retries": 187,
  "retry_success": 184,
  "circuit_breaker_open": false,
  "active_connections": 16,
  "cpu_percent": 34.2,
  "mem_rss_mb": 128.5
}
```

### Agent Port Assignments

| Agent | Port |
|-------|------|
| Java (library) | 7001 |
| Java (vanilla) | 7002 |
| Go (library) | 7003 |
| Go (vanilla) | 7004 |
| Python (library) | 7005 |
| Python (vanilla) | 7006 |
| .NET (library) | 7007 |
| .NET (vanilla) | 7008 |

## Dashboard

### Three Views

| View | Purpose |
|------|---------|
| Live Benchmark | Real-time workload tabs, chaos event overlays, all 8 runners per chart |
| Run History | Table of past runs, click to load into chart view |
| Compare | Side-by-side overlay of two historical runs |

### Metrics Displayed

- **Latency:** p50, p95, p99, max (microseconds)
- **Throughput:** ops/sec, total operations
- **Resources:** CPU %, memory RSS, active connections
- **Errors:** error count, retry count, retry success rate, circuit breaker state

### Chart Library

uPlot — handles high-frequency streaming across 8 runners efficiently.

## Result Storage

- **JSON files** — `results/<timestamp>-<topology>-<workload>.json` per run. Portable, git-trackable, shareable.
- **SQLite** — `benchmark.db` for historical queries, trend charts, run comparison. Powers the dashboard's history and compare views.

## Project Structure

```
cockroachdb-developer-benchmarks/
├── controller/
│   ├── cmd/server/main.go
│   ├── topology/
│   ├── scheduler/
│   ├── chaos/
│   ├── collector/
│   ├── api/
│   └── protocol/
├── agents/
│   ├── java/
│   │   ├── src/
│   │   ├── build.gradle
│   │   └── Dockerfile
│   ├── go/
│   │   ├── cmd/agent/main.go
│   │   ├── workloads/
│   │   └── Dockerfile
│   ├── python/
│   │   ├── agent/
│   │   ├── pyproject.toml
│   │   └── Dockerfile
│   └── dotnet/
│       ├── src/
│       ├── BenchmarkAgent.csproj
│       └── Dockerfile
├── dashboard/
│   ├── src/
│   │   ├── components/
│   │   ├── hooks/
│   │   └── App.tsx
│   ├── package.json
│   └── vite.config.ts
├── deploy/
│   ├── docker-compose.single.yml
│   ├── docker-compose.cluster.yml
│   ├── docker-compose.multi.yml
│   ├── docker-compose.agents.yml
│   └── init.sql
├── results/
├── benchmark.db
├── Makefile
└── README.md
```

## Makefile

```makefile
# Defaults
CONTROLLER_PORT ?= 9090
DASHBOARD_PORT  ?= 3000
TOPOLOGY        ?= single

# Lifecycle
make start                                               # Start controller + dashboard
make start CONTROLLER_PORT=9090 DASHBOARD_PORT=3001      # Custom ports
make start DB_URL="postgresql://..."                     # Remote CockroachDB
make stop                                                # Stop all (PID-file based)
make restart                                             # Stop + start
make dev                                                 # Dev mode with hot reload
make dev PORT=5679                                       # Dev mode, custom controller port

# Topology
make topo-up                          # Start CockroachDB (default: single)
make topo-up TOPOLOGY=cluster         # 3-node cluster
make topo-up TOPOLOGY=multi           # 9-node multi-region
make topo-down                        # Tear down CockroachDB

# Agents
make agents-up                        # Build & start all 8 agent containers
make agents-up AGENTS=go,python       # Specific languages only
make agents-down                      # Stop agents
make agents-build                     # Rebuild Docker images

# Benchmarks
make bench WORKLOAD=crud              # Single workload, all agents
make bench-all                        # All 6 workloads sequentially
make bench-chaos                      # All workloads + chaos gauntlet (9-node)

# Maintenance
make test                             # Run all agent unit tests
make clean                            # Remove containers, volumes, results
```

Process management uses PID files in `.pids/` for clean start/stop.

## Runner Execution Model

- **All-Docker (default):** All 8 agents run as Docker containers alongside CockroachDB. Reproducible, CI-friendly.
- **Local (`--local`):** Agents run on the host machine, CockroachDB stays in Docker. Faster iteration, requires local SDKs (JDK, Go, Python, .NET).
- **Remote:** No Docker topology. Pass `DB_URL` with connection string + credentials. Agents run locally or in Docker.
