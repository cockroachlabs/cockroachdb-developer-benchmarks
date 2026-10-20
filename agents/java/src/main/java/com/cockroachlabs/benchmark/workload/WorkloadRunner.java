package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.benchmark.protocol.WorkloadRequest;

public interface WorkloadRunner {

    void start(WorkloadRequest.WorkloadConfig config);

    void stop();

    boolean isRunning();

    MetricsRecorder getMetrics();
}
