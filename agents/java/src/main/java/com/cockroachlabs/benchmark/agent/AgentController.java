package com.cockroachlabs.benchmark.agent;

import com.cockroachlabs.benchmark.protocol.WorkloadRequest;
import com.cockroachlabs.benchmark.workload.WorkloadFactory;
import com.cockroachlabs.benchmark.workload.WorkloadRunner;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.Map;
import java.util.UUID;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;

@RestController
public class AgentController {

    private final WorkloadFactory factory;
    private final MetricsWebSocketHandler wsHandler;
    private final String mode;
    private final String language;

    private volatile WorkloadRunner currentRunner;
    private volatile String currentRunId;
    private ScheduledExecutorService metricsScheduler;

    public AgentController(WorkloadFactory factory, MetricsWebSocketHandler wsHandler,
                           @Value("${agent.mode}") String mode,
                           @Value("${agent.language:java}") String language) {
        this.factory = factory;
        this.wsHandler = wsHandler;
        this.mode = mode;
        this.language = language;
    }

    @PostMapping("/workload")
    public ResponseEntity<Map<String, String>> startWorkload(@RequestBody WorkloadRequest request) {
        if (currentRunner != null && currentRunner.isRunning()) {
            return ResponseEntity.badRequest().body(Map.of("status", "error", "message", "workload already running"));
        }

        String runId = UUID.randomUUID().toString();
        WorkloadRunner runner = factory.create(request.type());
        runner.getMetrics().setRunId(runId);
        runner.getMetrics().setLanguage(language);
        runner.start(request.config());

        currentRunner = runner;
        currentRunId = runId;

        metricsScheduler = Executors.newSingleThreadScheduledExecutor();
        metricsScheduler.scheduleAtFixedRate(() -> {
            if (runner.isRunning()) {
                wsHandler.broadcast(runner.getMetrics().snapshot());
            }
        }, 1, 1, TimeUnit.SECONDS);

        return ResponseEntity.ok(Map.of("run_id", runId, "status", "started"));
    }

    @PostMapping("/stop")
    public ResponseEntity<Map<String, String>> stopWorkload() {
        if (currentRunner == null || !currentRunner.isRunning()) {
            return ResponseEntity.ok(Map.of("status", "idle"));
        }
        currentRunner.stop();
        if (metricsScheduler != null) {
            metricsScheduler.shutdown();
            metricsScheduler = null;
        }
        currentRunner = null;
        currentRunId = null;
        return ResponseEntity.ok(Map.of("status", "stopped"));
    }

    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        return ResponseEntity.ok(Map.of(
                "status", "ok",
                "language", language,
                "mode", mode
        ));
    }

    @GetMapping("/info")
    public ResponseEntity<Map<String, String>> info() {
        return ResponseEntity.ok(Map.of(
                "language", language,
                "library_version", "spring-boot-3.3",
                "driver", "postgresql-42.7"
        ));
    }
}
