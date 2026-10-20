using System.Diagnostics;
using BenchmarkAgent.Protocol;

namespace BenchmarkAgent.Metrics;

public sealed class MetricsRecorder
{
    private readonly Lock _histLock = new();
    private readonly List<long> _latencies = new(4096);
    private long _opsTotal;
    private long _errors;
    private long _retries;
    private long _retrySuccess;
    private int _activeConnections;
    private long _lastOps;
    private long _lastSnapshotTicks = Stopwatch.GetTimestamp();
    private string _runId = "";
    private string _mode = "";
    private string _workload = "";
    private bool _circuitBreakerOpen;

    public void SetContext(string runId, string mode, string workload)
    {
        _runId = runId;
        _mode = mode;
        _workload = workload;
    }

    public void RecordOp(long latencyUs)
    {
        Interlocked.Increment(ref _opsTotal);
        lock (_histLock)
        {
            _latencies.Add(Math.Clamp(latencyUs, 0, 60_000_000));
        }
    }

    public void RecordError() => Interlocked.Increment(ref _errors);

    public void RecordRetry() => Interlocked.Increment(ref _retries);

    public void RecordRetrySuccess() => Interlocked.Increment(ref _retrySuccess);

    public void SetActiveConnections(int count) =>
        Interlocked.Exchange(ref _activeConnections, count);

    public void SetCircuitBreakerOpen(bool open) => _circuitBreakerOpen = open;

    private static double Percentile(List<long> sorted, double p)
    {
        if (sorted.Count == 0) return 0;
        var idx = (int)(sorted.Count * p / 100.0);
        return sorted[Math.Min(idx, sorted.Count - 1)];
    }

    public MetricSnapshot Snapshot()
    {
        var now = Stopwatch.GetTimestamp();
        var currentOps = Interlocked.Read(ref _opsTotal);
        var elapsed = Stopwatch.GetElapsedTime(_lastSnapshotTicks, now).TotalSeconds;
        var opsPerSec = elapsed > 0 ? (currentOps - _lastOps) / elapsed : 0;

        double p50, p95, p99, max;
        lock (_histLock)
        {
            _latencies.Sort();
            p50 = Percentile(_latencies, 50.0);
            p95 = Percentile(_latencies, 95.0);
            p99 = Percentile(_latencies, 99.0);
            max = _latencies.Count > 0 ? _latencies[^1] : 0;
            _latencies.Clear();
        }

        _lastOps = currentOps;
        _lastSnapshotTicks = now;

        var proc = Process.GetCurrentProcess();
        var cpuPercent = proc.TotalProcessorTime.TotalMilliseconds /
                         (Environment.ProcessorCount * (DateTime.UtcNow - proc.StartTime.ToUniversalTime()).TotalMilliseconds) * 100;
        var memMb = proc.WorkingSet64 / (1024.0 * 1024.0);

        return new MetricSnapshot(
            Ts: DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(),
            RunId: _runId,
            Language: "dotnet",
            Mode: _mode,
            Workload: _workload,
            OpsTotal: currentOps,
            OpsPerSec: opsPerSec,
            LatencyP50Us: p50,
            LatencyP95Us: p95,
            LatencyP99Us: p99,
            LatencyMaxUs: max,
            Errors: Interlocked.Read(ref _errors),
            Retries: Interlocked.Read(ref _retries),
            RetrySuccess: Interlocked.Read(ref _retrySuccess),
            CircuitBreakerOpen: _circuitBreakerOpen,
            ActiveConnections: Volatile.Read(ref _activeConnections),
            CpuPercent: Math.Max(0, cpuPercent),
            MemRssMb: memMb
        );
    }

    public void Reset()
    {
        Interlocked.Exchange(ref _opsTotal, 0);
        Interlocked.Exchange(ref _errors, 0);
        Interlocked.Exchange(ref _retries, 0);
        Interlocked.Exchange(ref _retrySuccess, 0);
        lock (_histLock) { _latencies.Clear(); }
        _lastOps = 0;
        _lastSnapshotTicks = Stopwatch.GetTimestamp();
    }
}
