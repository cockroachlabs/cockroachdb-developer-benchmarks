# Project Status

**Last updated:** 2026-06-09

## What Works

| Component | Status | Details |
|-----------|:------:|---------|
| **Go Controller** | Working | Topology manager, scheduler, chaos engine, metrics collector, API/WebSocket server on port 9495 |
| **Go Agent (library)** | Working | Uses real `crdbgo/pgxadapter.Pool`, `ExecuteTx`, `QueryFollowerRead`, `cdc.WebhookHandler`, `ai.VectorSearchTemplate` |
| **Go Agent (vanilla)** | Working | Raw pgx/v5, manual retry, raw SQL |
| **Python Agent (library)** | Working | Uses real `CockroachDBSession`, `@crdb_retry`, `cockroachdb://` SQLAlchemy dialect, `VectorSearchTemplate` |
| **Python Agent (vanilla)** | Working | Raw psycopg3, manual retry |
| **.NET Agent (vanilla)** | Working | Raw Npgsql, manual retry |
| **React Dashboard** | Partial | Layout, controls, workload tabs, agent status bar with live ops/sec all work. Chart lines not rendering (see Known Issues). |
| **WebSocket Pipeline** | Working | Controller collects metrics from agents and broadcasts to dashboard. Confirmed 75 msgs/15s per agent. |
| **HTML Report Generator** | Ready | `GET /api/runs/:id/report` endpoint, `make report` target. Not yet tested with a full run. |
| **Docker Compose (single)** | Working | CockroachDB v25.1 single-node with benchmark database and schema |
| **Docker Compose (cluster)** | Ready | 3-node cluster, not yet tested |
| **Docker Compose (multi)** | Ready | 9-node multi-region with localities, not yet tested |
| **Makefile** | Working | All targets functional, auto-detects `docker-compose` vs `docker compose` |

## Known Issues

### 1. Chart lines not rendering (Dashboard)

**Impact:** High — the live chart area shows correct timestamps and axis but no data lines.

**Root cause:** Agents stream metrics at different rates. The .NET vanilla agent (idle) sends data every second while Go/Python agents send every ~5 seconds. The `extractSeries` function creates aligned timestamps from all agents, but sparse agents have mostly NaN values. Even with `spanGaps: true`, the lines are not visible because the Y scale is dominated by 0-value idle data.

**Fix needed:** Filter idle agents (0 ops/s) from the timeline alignment, or interpolate sparse data points, or use a per-agent time series instead of aligned data.

### 2. Java agents return 400 Bad Request

**Impact:** High — Java agents cannot execute workloads.

**Root cause:** The controller sends `db_url: ""` (empty string) in the workload config. Java's `WorkloadRequest` record has primitive `int` fields (`durationSecs`, `concurrency`) that Jackson cannot deserialize from JSON `null`. Error: `Cannot map null into type int`.

**Fix needed:** Either change Java records to use `Integer` (boxed types), or ensure the controller always sends non-null values for all fields.

### 3. .NET library agent crashes on startup

**Impact:** Medium — .NET vanilla agent works, but library mode does not.

**Root cause:** The `DB_URL` environment variable uses PostgreSQL URL format (`postgresql://root@crdb-1:26257/benchmark?sslmode=disable`). Npgsql's `NpgsqlConnectionStringBuilder` expects ADO.NET connection string format (`Host=crdb-1;Port=26257;Database=benchmark;Username=root;SSL Mode=Disable`). The library agent's `Program.cs` passes the raw URL to `NpgsqlDataSourceBuilder`.

**Fix needed:** Add a URL-to-connection-string converter in `Program.cs`, or set a second env var `DB_CONNECTION_STRING` in the docker-compose.

### 4. Go agents show 0 ops/s during workloads

**Impact:** Medium — Go agents accept and run workloads but metrics show 0 operations.

**Root cause:** Under investigation. The agents start workloads successfully (no errors in logs) and the controller receives metric snapshots from them, but `ops_per_sec` is always 0. Likely the workload goroutines are failing silently or the metrics recorder isn't being called from the worker functions.

**Fix needed:** Add error logging in Go agent workload workers to surface silent failures.

## Architecture Verified

The end-to-end architecture is proven:

```
Dashboard (React) ←ws→ Controller (Go:9495) ←ws+http→ Agents (Docker:7001-7008) ←pg→ CockroachDB (Docker:26257)
```

- Controller dispatches workloads to agents via HTTP POST
- Agents execute workloads and stream metrics via WebSocket
- Controller aggregates metrics and broadcasts to dashboard subscribers
- Dashboard receives and displays live data in the agent status bar
- SQLite stores historical run data
- HTML report generator produces self-contained shareable reports

## File Counts

| Component | Files | Lines |
|-----------|------:|------:|
| Controller (Go) | 10 | ~2,100 |
| Go Agent | 10 | ~1,700 |
| Java Agent | 19 | ~1,200 |
| Python Agent | 15 | ~1,000 |
| .NET Agent | 17 | ~1,300 |
| Dashboard (React/TS) | 17 | ~1,500 |
| Deploy (Docker) | 5 | ~400 |
| Docs + Config | 8 | ~800 |
| **Total** | **~101** | **~10,000** |

## Next Steps

1. Fix chart rendering (highest priority — the visual payoff)
2. Fix Java 400 error (boxed Integer types or controller sends defaults)
3. Fix .NET library agent connection string
4. Debug Go agent 0 ops/s
5. Run full benchmark suite and generate HTML report
6. Add benchmark warmup phase (10-15s before measuring)
7. Run 3-node cluster benchmarks with follower reads
8. Run 9-node multi-region with chaos engineering
