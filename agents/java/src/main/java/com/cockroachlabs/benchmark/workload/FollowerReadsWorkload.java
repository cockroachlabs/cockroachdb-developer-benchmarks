package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.benchmark.protocol.WorkloadRequest;
import com.cockroachlabs.data.template.CockroachDBTemplate;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;
import java.sql.*;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicBoolean;

public class FollowerReadsWorkload implements WorkloadRunner {

    private final DataSource dataSource;
    private final String mode;
    private final CockroachDBTemplate template;
    private final MetricsRecorder metrics = new MetricsRecorder();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ExecutorService executor;

    private static final String READ_QUERY =
            "SELECT id, balance, name, region FROM bench_accounts " +
            "WHERE region = ? ORDER BY updated_at DESC LIMIT 50";

    private static final String VANILLA_QUERY =
            "SELECT id, balance, name, region FROM bench_accounts " +
            "WHERE region = ? ORDER BY updated_at DESC LIMIT 50";

    private static final String[] REGIONS = {"us-east", "us-west", "eu-west"};

    public FollowerReadsWorkload(DataSource dataSource, String mode, CockroachDBTemplate template) {
        this.dataSource = dataSource;
        this.mode = mode;
        this.template = template;
    }

    @Override
    public void start(WorkloadRequest.WorkloadConfig config) {
        if (running.getAndSet(true)) return;
        metrics.reset();
        metrics.setMode(mode);
        metrics.setWorkload("follower_reads");
        int concurrency = config.concurrency() > 0 ? config.concurrency() : 8;
        executor = Executors.newVirtualThreadPerTaskExecutor();
        for (int i = 0; i < concurrency; i++) {
            executor.submit(() -> runLoop(config));
        }
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
        JdbcTemplate jdbc = new JdbcTemplate(dataSource);
        while (running.get() && System.currentTimeMillis() < deadline) {
            try {
                long start = System.nanoTime();
                String region = REGIONS[ThreadLocalRandom.current().nextInt(REGIONS.length)];
                template.executeReadCommitted(() -> {
                    jdbc.execute("SET TRANSACTION AS OF SYSTEM TIME follower_read_timestamp()");
                    return jdbc.queryForList(READ_QUERY, region);
                });
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
                String region = REGIONS[ThreadLocalRandom.current().nextInt(REGIONS.length)];

                try (Statement st = conn.createStatement()) {
                    st.execute("SET TRANSACTION AS OF SYSTEM TIME follower_read_timestamp()");
                }
                try (PreparedStatement ps = conn.prepareStatement(VANILLA_QUERY)) {
                    ps.setString(1, region);
                    ps.executeQuery().close();
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
