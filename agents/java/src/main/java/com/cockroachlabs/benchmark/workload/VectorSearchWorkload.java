package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.benchmark.protocol.WorkloadRequest;
import com.cockroachlabs.data.ai.DistanceMetric;
import com.cockroachlabs.data.ai.VectorSearchTemplate;

import javax.sql.DataSource;
import java.sql.*;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

public class VectorSearchWorkload implements WorkloadRunner {

    private static final int VECTOR_DIM = 1536;
    private final DataSource dataSource;
    private final String mode;
    private final VectorSearchTemplate vectorTemplate;
    private final MetricsRecorder metrics = new MetricsRecorder();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ExecutorService executor;

    public VectorSearchWorkload(DataSource dataSource, String mode, VectorSearchTemplate vectorTemplate) {
        this.dataSource = dataSource;
        this.mode = mode;
        this.vectorTemplate = vectorTemplate;
    }

    @Override
    public void start(WorkloadRequest.WorkloadConfig config) {
        if (running.getAndSet(true)) return;
        metrics.reset();
        metrics.setMode(mode);
        metrics.setWorkload("vector_search");
        int concurrency = config.concurrency() > 0 ? config.concurrency() : 4;
        executor = Executors.newVirtualThreadPerTaskExecutor();
        for (int i = 0; i < concurrency; i++) {
            executor.submit(() -> runLoop(config));
        }
    }

    private float[] randomFloatVector() {
        float[] vec = new float[VECTOR_DIM];
        for (int i = 0; i < VECTOR_DIM; i++) {
            vec[i] = (float) ThreadLocalRandom.current().nextDouble(-1, 1);
        }
        return vec;
    }

    private String randomVectorString() {
        return "[" + IntStream.range(0, VECTOR_DIM)
                .mapToObj(i -> String.format("%.6f", ThreadLocalRandom.current().nextDouble(-1, 1)))
                .collect(Collectors.joining(",")) + "]";
    }

    private void runLoop(WorkloadRequest.WorkloadConfig config) {
        long deadline = System.currentTimeMillis() + (config.durationSecs() * 1000L);
        if ("library".equals(mode)) {
            runLibraryLoop(deadline);
        } else {
            runVanillaLoop(deadline);
        }
    }

    private void runLibraryLoop(long deadline) {
        while (running.get() && System.currentTimeMillis() < deadline) {
            try {
                long start = System.nanoTime();
                boolean isInsert = ThreadLocalRandom.current().nextInt(100) < 30;

                if (isInsert) {
                    String id = UUID.randomUUID().toString();
                    float[] embedding = randomFloatVector();
                    vectorTemplate.storeEmbedding(
                            "bench_documents", "id", id,
                            "embedding", embedding);
                } else {
                    float[] queryVec = randomFloatVector();
                    vectorTemplate.search(
                            "bench_documents", "embedding",
                            queryVec, DistanceMetric.COSINE, 5);
                }

                long elapsed = (System.nanoTime() - start) / 1000;
                metrics.recordOp(elapsed);
            } catch (Exception e) {
                metrics.recordError();
            }
        }
    }

    private void runVanillaLoop(long deadline) {
        while (running.get() && System.currentTimeMillis() < deadline) {
            try (Connection conn = dataSource.getConnection()) {
                long start = System.nanoTime();
                boolean isInsert = ThreadLocalRandom.current().nextInt(100) < 30;

                if (isInsert) {
                    try (PreparedStatement ps = conn.prepareStatement(
                            "INSERT INTO bench_documents (id, content, embedding, metadata) " +
                            "VALUES (?::UUID, ?, ?::VECTOR, ?::JSONB)")) {
                        ps.setString(1, UUID.randomUUID().toString());
                        ps.setString(2, "doc-" + ThreadLocalRandom.current().nextInt(100000));
                        ps.setString(3, randomVectorString());
                        ps.setString(4, "{\"source\": \"benchmark\"}");
                        ps.executeUpdate();
                    }
                } else {
                    String queryVec = randomVectorString();
                    try (PreparedStatement ps = conn.prepareStatement(
                            "SELECT id, content, 1 - (embedding <=> ?::VECTOR) AS similarity " +
                            "FROM bench_documents ORDER BY embedding <=> ?::VECTOR LIMIT 5")) {
                        ps.setString(1, queryVec);
                        ps.setString(2, queryVec);
                        ps.executeQuery().close();
                    }
                }

                long elapsed = (System.nanoTime() - start) / 1000;
                metrics.recordOp(elapsed);
            } catch (Exception e) {
                metrics.recordError();
            }
        }
    }

    @Override
    public void stop() {
        running.set(false);
        if (executor != null) executor.shutdown();
    }

    @Override
    public boolean isRunning() { return running.get(); }

    @Override
    public MetricsRecorder getMetrics() { return metrics; }
}
