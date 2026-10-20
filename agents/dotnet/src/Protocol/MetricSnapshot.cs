using System.Text.Json.Serialization;

namespace BenchmarkAgent.Protocol;

public record MetricSnapshot(
    [property: JsonPropertyName("ts")] long Ts,
    [property: JsonPropertyName("run_id")] string RunId,
    [property: JsonPropertyName("language")] string Language,
    [property: JsonPropertyName("mode")] string Mode,
    [property: JsonPropertyName("workload")] string Workload,
    [property: JsonPropertyName("ops_total")] long OpsTotal,
    [property: JsonPropertyName("ops_per_sec")] double OpsPerSec,
    [property: JsonPropertyName("latency_p50_us")] double LatencyP50Us,
    [property: JsonPropertyName("latency_p95_us")] double LatencyP95Us,
    [property: JsonPropertyName("latency_p99_us")] double LatencyP99Us,
    [property: JsonPropertyName("latency_max_us")] double LatencyMaxUs,
    [property: JsonPropertyName("errors")] long Errors,
    [property: JsonPropertyName("retries")] long Retries,
    [property: JsonPropertyName("retry_success")] long RetrySuccess,
    [property: JsonPropertyName("circuit_breaker_open")] bool CircuitBreakerOpen,
    [property: JsonPropertyName("active_connections")] int ActiveConnections,
    [property: JsonPropertyName("cpu_percent")] double CpuPercent,
    [property: JsonPropertyName("mem_rss_mb")] double MemRssMb
);
