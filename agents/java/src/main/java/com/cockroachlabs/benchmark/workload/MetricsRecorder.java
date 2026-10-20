package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.benchmark.protocol.MetricSnapshot;
import org.HdrHistogram.Histogram;

import java.lang.management.ManagementFactory;
import java.lang.management.OperatingSystemMXBean;
import java.util.concurrent.atomic.AtomicLong;

public class MetricsRecorder {

    private final AtomicLong opsTotal = new AtomicLong();
    private final AtomicLong errors = new AtomicLong();
    private final AtomicLong retries = new AtomicLong();
    private final AtomicLong retrySuccess = new AtomicLong();
    private final Histogram histogram = new Histogram(60_000_000L, 3);
    private final OperatingSystemMXBean osMxBean = ManagementFactory.getOperatingSystemMXBean();

    private volatile boolean circuitBreakerOpen = false;
    private volatile int activeConnections = 0;
    private volatile String runId = "";
    private volatile String language = "java";
    private volatile String mode = "";
    private volatile String workload = "";

    private long lastOps = 0;
    private long lastSnapshotTime = System.nanoTime();

    public void recordOp(long latencyUs) {
        opsTotal.incrementAndGet();
        synchronized (histogram) {
            histogram.recordValue(Math.min(latencyUs, 60_000_000L));
        }
    }

    public void recordError() {
        errors.incrementAndGet();
    }

    public void recordRetry(boolean succeeded) {
        retries.incrementAndGet();
        if (succeeded) {
            retrySuccess.incrementAndGet();
        }
    }

    public void setRunId(String runId) { this.runId = runId; }
    public void setLanguage(String language) { this.language = language; }
    public void setMode(String mode) { this.mode = mode; }
    public void setWorkload(String workload) { this.workload = workload; }
    public void setCircuitBreakerOpen(boolean open) { this.circuitBreakerOpen = open; }
    public void setActiveConnections(int count) { this.activeConnections = count; }

    public MetricSnapshot snapshot() {
        long now = System.nanoTime();
        long currentOps = opsTotal.get();
        double elapsed = (now - lastSnapshotTime) / 1_000_000_000.0;
        double opsPerSec = elapsed > 0 ? (currentOps - lastOps) / elapsed : 0;
        lastOps = currentOps;
        lastSnapshotTime = now;

        double p50, p95, p99, max;
        synchronized (histogram) {
            p50 = histogram.getValueAtPercentile(50.0);
            p95 = histogram.getValueAtPercentile(95.0);
            p99 = histogram.getValueAtPercentile(99.0);
            max = histogram.getMaxValue();
        }

        double cpuLoad = osMxBean.getSystemLoadAverage();
        if (cpuLoad < 0) cpuLoad = 0;
        Runtime rt = Runtime.getRuntime();
        double memMb = (rt.totalMemory() - rt.freeMemory()) / (1024.0 * 1024.0);

        return new MetricSnapshot(
                System.currentTimeMillis(),
                runId, language, mode, workload,
                currentOps, opsPerSec,
                p50, p95, p99, max,
                errors.get(), retries.get(), retrySuccess.get(),
                circuitBreakerOpen, activeConnections,
                cpuLoad, memMb
        );
    }

    public void reset() {
        opsTotal.set(0);
        errors.set(0);
        retries.set(0);
        retrySuccess.set(0);
        synchronized (histogram) {
            histogram.reset();
        }
        lastOps = 0;
        lastSnapshotTime = System.nanoTime();
    }
}
