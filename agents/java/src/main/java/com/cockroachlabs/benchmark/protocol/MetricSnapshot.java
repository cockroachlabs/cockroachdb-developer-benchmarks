package com.cockroachlabs.benchmark.protocol;

import com.fasterxml.jackson.annotation.JsonProperty;

public record MetricSnapshot(
        long ts,
        @JsonProperty("run_id") String runId,
        String language,
        String mode,
        String workload,
        @JsonProperty("ops_total") long opsTotal,
        @JsonProperty("ops_per_sec") double opsPerSec,
        @JsonProperty("latency_p50_us") double latencyP50Us,
        @JsonProperty("latency_p95_us") double latencyP95Us,
        @JsonProperty("latency_p99_us") double latencyP99Us,
        @JsonProperty("latency_max_us") double latencyMaxUs,
        long errors,
        long retries,
        @JsonProperty("retry_success") long retrySuccess,
        @JsonProperty("circuit_breaker_open") boolean circuitBreakerOpen,
        @JsonProperty("active_connections") int activeConnections,
        @JsonProperty("cpu_percent") double cpuPercent,
        @JsonProperty("mem_rss_mb") double memRssMb
) {}
