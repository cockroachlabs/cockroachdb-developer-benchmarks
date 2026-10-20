package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.benchmark.protocol.WorkloadRequest;
import com.cockroachlabs.data.template.CockroachDBTemplate;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;
import java.math.BigDecimal;
import java.sql.*;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicBoolean;

public class CdcWorkload implements WorkloadRunner {

    private final DataSource dataSource;
    private final String mode;
    private final CockroachDBTemplate template;
    private final MetricsRecorder metrics = new MetricsRecorder();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ExecutorService executor;

    public CdcWorkload(DataSource dataSource, String mode, CockroachDBTemplate template) {
        this.dataSource = dataSource;
        this.mode = mode;
        this.template = template;
    }

    @Override
    public void start(WorkloadRequest.WorkloadConfig config) {
        if (running.getAndSet(true)) return;
        metrics.reset();
        metrics.setMode(mode);
        metrics.setWorkload("cdc");
        int concurrency = config.concurrency() > 0 ? config.concurrency() : 4;
        executor = Executors.newVirtualThreadPerTaskExecutor();

        for (int i = 0; i < concurrency; i++) {
            executor.submit(() -> runWriterLoop(config));
        }
        executor.submit(() -> runConsumerLoop(config));
    }

    private void runWriterLoop(WorkloadRequest.WorkloadConfig config) {
        long deadline = System.currentTimeMillis() + (config.durationSecs() * 1000L);
        if ("library".equals(mode)) {
            runLibraryWriter(deadline);
        } else {
            runVanillaWriter(deadline);
        }
    }

    private void runLibraryWriter(long deadline) {
        JdbcTemplate jdbc = new JdbcTemplate(dataSource);
        while (running.get() && System.currentTimeMillis() < deadline) {
            try {
                long start = System.nanoTime();
                String id = UUID.randomUUID().toString();
                BigDecimal balance = BigDecimal.valueOf(ThreadLocalRandom.current().nextDouble(1000, 10000));
                String name = "cdc-" + ThreadLocalRandom.current().nextInt(100000);
                template.executeWithoutResult(() ->
                    jdbc.update("INSERT INTO bench_accounts (id, balance, name, region) VALUES (?::UUID, ?, ?, ?)",
                            id, balance, name, "us-east")
                );
                long elapsed = (System.nanoTime() - start) / 1000;
                metrics.recordOp(elapsed);
            } catch (Exception e) {
                metrics.recordError();
            }
        }
    }

    private void runVanillaWriter(long deadline) {
        while (running.get() && System.currentTimeMillis() < deadline) {
            try (Connection conn = dataSource.getConnection();
                 PreparedStatement ps = conn.prepareStatement(
                         "INSERT INTO bench_accounts (id, balance, name, region) VALUES (?::UUID, ?, ?, ?)")) {
                long start = System.nanoTime();
                ps.setString(1, UUID.randomUUID().toString());
                ps.setBigDecimal(2, BigDecimal.valueOf(ThreadLocalRandom.current().nextDouble(1000, 10000)));
                ps.setString(3, "cdc-" + ThreadLocalRandom.current().nextInt(100000));
                ps.setString(4, "us-east");
                ps.executeUpdate();
                long elapsed = (System.nanoTime() - start) / 1000;
                metrics.recordOp(elapsed);
            } catch (Exception e) {
                metrics.recordError();
            }
        }
    }

    private void runConsumerLoop(WorkloadRequest.WorkloadConfig config) {
        long deadline = System.currentTimeMillis() + (config.durationSecs() * 1000L);
        if ("library".equals(mode)) {
            runLibraryConsumer(deadline);
        } else {
            runVanillaConsumer(deadline);
        }
    }

    private void runLibraryConsumer(long deadline) {
        JdbcTemplate jdbc = new JdbcTemplate(dataSource);
        while (running.get() && System.currentTimeMillis() < deadline) {
            try {
                template.execute(() -> {
                    var recent = jdbc.queryForList(
                            "SELECT id, name, created_at FROM bench_accounts WHERE name LIKE 'cdc-%' ORDER BY created_at DESC LIMIT 10");
                    for (var row : recent) {
                        Timestamp createdAt = (Timestamp) row.get("created_at");
                        long lagMs = System.currentTimeMillis() - createdAt.getTime();
                        jdbc.update("INSERT INTO bench_cdc_events (id, source_table, operation, payload, lag_ms) " +
                                        "VALUES (?::UUID, 'bench_accounts', 'INSERT', ?::JSONB, ?)",
                                UUID.randomUUID().toString(),
                                "{\"account_id\": \"" + row.get("id") + "\"}",
                                lagMs);
                    }
                    return null;
                });
                Thread.sleep(100);
            } catch (Exception e) {
                metrics.recordError();
            }
        }
    }

    private void runVanillaConsumer(long deadline) {
        while (running.get() && System.currentTimeMillis() < deadline) {
            try (Connection conn = dataSource.getConnection()) {
                try (PreparedStatement ps = conn.prepareStatement(
                        "SELECT id, name, created_at FROM bench_accounts WHERE name LIKE 'cdc-%' ORDER BY created_at DESC LIMIT 10")) {
                    ResultSet rs = ps.executeQuery();
                    while (rs.next()) {
                        Timestamp createdAt = rs.getTimestamp("created_at");
                        long lagMs = System.currentTimeMillis() - createdAt.getTime();
                        try (PreparedStatement insert = conn.prepareStatement(
                                "INSERT INTO bench_cdc_events (id, source_table, operation, payload, lag_ms) " +
                                "VALUES (?::UUID, 'bench_accounts', 'INSERT', ?::JSONB, ?)")) {
                            insert.setString(1, UUID.randomUUID().toString());
                            insert.setString(2, "{\"account_id\": \"" + rs.getString("id") + "\"}");
                            insert.setLong(3, lagMs);
                            insert.executeUpdate();
                        }
                    }
                }
                Thread.sleep(100);
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
